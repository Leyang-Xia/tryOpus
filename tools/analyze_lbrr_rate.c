#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <opus/opus.h>

#include "../src/wav_io.h"

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "用法: %s <input.wav> [bitrate] [plp] [vbr=0|1]\n", argv[0]);
        return 1;
    }

    const char *input = argv[1];
    int bitrate = argc > 2 ? atoi(argv[2]) : 32000;
    int plp     = argc > 3 ? atoi(argv[3]) : 10;
    int vbr     = argc > 4 ? atoi(argv[4]) : 0;

    WAVFile wf;
    if (wav_open_read(&wf, input) < 0) return 1;

    int sr = (int)wf.hdr.sample_rate;
    int ch = (int)wf.hdr.num_channels;
    int frame_ms = 20;
    int frame_samples = sr * frame_ms / 1000;

    int err;
    OpusEncoder *enc = opus_encoder_create(sr, ch, OPUS_APPLICATION_VOIP, &err);
    opus_encoder_ctl(enc, OPUS_SET_BITRATE(bitrate));
    opus_encoder_ctl(enc, OPUS_SET_COMPLEXITY(9));
    opus_encoder_ctl(enc, OPUS_SET_INBAND_FEC(1));
    opus_encoder_ctl(enc, OPUS_SET_PACKET_LOSS_PERC(plp));
    opus_encoder_ctl(enc, OPUS_SET_VBR(vbr));

    int16_t *pcm = calloc(frame_samples * ch, sizeof(int16_t));
    uint8_t *pkt = malloc(4000);

    int total = 0, has_lbrr = 0, silk_frames = 0, celt_frames = 0;
    int bandwidth_counts[6] = {0};
    int speech_too_low = 0;

    while (1) {
        int nread = wav_read_samples(&wf, pcm, frame_samples);
        if (nread <= 0) break;
        if (nread < frame_samples)
            memset(pcm + nread * ch, 0, (frame_samples - nread) * ch * sizeof(int16_t));

        int nbytes = opus_encode(enc, pcm, frame_samples, pkt, 4000);
        if (nbytes < 0) break;

        total++;

        if (nbytes > 1) {
            int lbrr = opus_packet_has_lbrr(pkt, nbytes);
            if (lbrr) has_lbrr++;

            int bw = opus_packet_get_bandwidth(pkt);
            int bw_idx = bw - OPUS_BANDWIDTH_NARROWBAND;
            if (bw_idx >= 0 && bw_idx < 6) bandwidth_counts[bw_idx]++;

            int mode = opus_packet_get_nb_channels(pkt);
            (void)mode;

            unsigned char toc = pkt[0];
            int config = toc >> 3;
            if (config < 12) silk_frames++;
            else if (config < 16) { /* hybrid */ silk_frames++; }
            else celt_frames++;
        }
    }

    wav_close_read(&wf);

    const char *bw_names[] = {"NB", "MB", "WB", "SWB", "FB", "?"};

    printf("=== LBRR 生成率分析 ===\n");
    printf("输入        : %s\n", input);
    printf("码率        : %d bps (%s)\n", bitrate, vbr ? "VBR" : "CBR");
    printf("声明丢包率  : %d%%\n", plp);
    printf("总帧数      : %d\n", total);
    printf("含LBRR帧数  : %d\n", has_lbrr);
    printf("LBRR生成率  : %.1f%%\n", total > 0 ? 100.0 * has_lbrr / total : 0);
    printf("SILK帧数    : %d (%.1f%%)\n", silk_frames, total > 0 ? 100.0 * silk_frames / total : 0);
    printf("CELT帧数    : %d (%.1f%%)\n", celt_frames, total > 0 ? 100.0 * celt_frames / total : 0);
    printf("带宽分布    :");
    for (int i = 0; i < 5; i++) {
        if (bandwidth_counts[i] > 0)
            printf(" %s=%d(%.0f%%)", bw_names[i], bandwidth_counts[i],
                   100.0 * bandwidth_counts[i] / total);
    }
    printf("\n");

    opus_encoder_destroy(enc);
    free(pcm);
    free(pkt);
    return 0;
}
