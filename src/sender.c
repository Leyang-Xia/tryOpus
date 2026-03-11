/**
 * sender.c - Opus UDP 发送端（实时流式传输）
 *
 * 读取WAV文件，Opus编码后通过UDP发往接收端，
 * 内置软件网络仿真模块（丢包/抖动/时延）。
 *
 * 用法: opus_sender [选项] input.wav
 *   -h <ip>    接收端IP (默认: 127.0.0.1)
 *   -p <port>  接收端端口 (默认: 5004)
 *   -b <bps>   码率 (默认: 32000)
 *   -fs <ms>   帧长 (默认: 20)
 *   -fec       开启LBRR
 *   -dred <n>  DRED冗余帧数
 *   -dtx       开启DTX
 *   -l <rate>  均匀丢包率 [0,1]
 *   -ge        Gilbert-Elliott丢包模型
 *   -j <ms>    抖动标准差 (ms)
 *   -d <ms>    固定时延 (ms)
 *   -speed <f> 加速倍速发送 (默认:1.0 = 实时)
 *   -v         详细输出
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <time.h>
#include <unistd.h>
#include <errno.h>
#include <sys/time.h>
#include <arpa/inet.h>
#include <sys/socket.h>
#include <netinet/in.h>

#include "common.h"
#include "wav_io.h"
#include "netsim.h"
#include <opus/opus.h>

static void *load_blob(const char *path, int *out_len) {
    FILE *f = fopen(path, "rb");
    if (!f) return NULL;
    fseek(f, 0, SEEK_END);
    long len = ftell(f);
    fseek(f, 0, SEEK_SET);
    void *buf = malloc((size_t)len);
    if (!buf) { fclose(f); return NULL; }
    fread(buf, 1, (size_t)len, f);
    fclose(f);
    *out_len = (int)len;
    return buf;
}

/* 高精度睡眠（微秒级） */
static void usleep_accurate(long us) {
    struct timespec ts;
    ts.tv_sec  = us / 1000000;
    ts.tv_nsec = (us % 1000000) * 1000;
    nanosleep(&ts, NULL);
}

/* 获取当前时间（微秒） */
static int64_t now_us(void) {
    struct timeval tv;
    gettimeofday(&tv, NULL);
    return (int64_t)tv.tv_sec * 1000000 + tv.tv_usec;
}

typedef struct {
    char    host[64];
    int     port;
    char    input_wav[256];
    EncoderConfig enc_cfg;
    NetSimConfig  net_cfg;
    double  speed;        /* 加速比 */
    int     verbose;
} SenderConfig;

static void parse_sender_args(int argc, char **argv, SenderConfig *cfg) {
    strcpy(cfg->host, "127.0.0.1");
    cfg->port   = DEFAULT_PORT;
    cfg->speed  = 1.0;
    cfg->verbose = 0;

    cfg->enc_cfg.sample_rate      = DEFAULT_SAMPLE_RATE;
    cfg->enc_cfg.channels         = DEFAULT_CHANNELS;
    cfg->enc_cfg.application      = OPUS_APPLICATION_VOIP;
    cfg->enc_cfg.bitrate          = 32000;
    cfg->enc_cfg.frame_size_ms    = 20;
    cfg->enc_cfg.use_fec          = 0;
    cfg->enc_cfg.packet_loss_perc = 0;
    cfg->enc_cfg.use_dtx          = 0;
    cfg->enc_cfg.use_vbr          = 0;
    cfg->enc_cfg.dred_duration    = 0;
    cfg->enc_cfg.complexity       = 9;

    cfg->net_cfg.loss_model    = LOSS_UNIFORM;
    cfg->net_cfg.loss_rate     = 0.0f;
    cfg->net_cfg.p_good_to_bad = 0.05f;
    cfg->net_cfg.p_bad_to_good = 0.30f;
    cfg->net_cfg.loss_in_bad   = 0.80f;
    cfg->net_cfg.loss_in_good  = 0.02f;
    cfg->net_cfg.base_delay_ms = 0.0f;
    cfg->net_cfg.jitter_std_ms = 0.0f;

    for (int i = 1; i < argc; i++) {
        if      (!strcmp(argv[i], "-h"))    strncpy(cfg->host, argv[++i], 63);
        else if (!strcmp(argv[i], "-p"))    cfg->port = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-b"))    cfg->enc_cfg.bitrate = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-fs"))   cfg->enc_cfg.frame_size_ms = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-fec"))  cfg->enc_cfg.use_fec = 1;
        else if (!strcmp(argv[i], "-plp"))  cfg->enc_cfg.packet_loss_perc = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-dred")) cfg->enc_cfg.dred_duration = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-dtx"))  cfg->enc_cfg.use_dtx = 1;
        else if (!strcmp(argv[i], "-vbr"))  cfg->enc_cfg.use_vbr = 1;
        else if (!strcmp(argv[i], "-l"))    cfg->net_cfg.loss_rate = (float)atof(argv[++i]);
        else if (!strcmp(argv[i], "-ge"))   cfg->net_cfg.loss_model = LOSS_GILBERT;
        else if (!strcmp(argv[i], "-ge-p2b")) cfg->net_cfg.p_good_to_bad = (float)atof(argv[++i]);
        else if (!strcmp(argv[i], "-ge-b2g")) cfg->net_cfg.p_bad_to_good = (float)atof(argv[++i]);
        else if (!strcmp(argv[i], "-ge-bloss")) cfg->net_cfg.loss_in_bad = (float)atof(argv[++i]);
        else if (!strcmp(argv[i], "-j"))    cfg->net_cfg.jitter_std_ms = (float)atof(argv[++i]);
        else if (!strcmp(argv[i], "-d"))    cfg->net_cfg.base_delay_ms = (float)atof(argv[++i]);
        else if (!strcmp(argv[i], "-speed")) cfg->speed = atof(argv[++i]);
        else if (!strcmp(argv[i], "-v"))    cfg->verbose = 1;
        else if (argv[i][0] != '-')
            strncpy(cfg->input_wav, argv[i], sizeof(cfg->input_wav) - 1);
    }
}

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "用法: %s [选项] input.wav\n"
                "      使用 -v 查看详细选项\n", argv[0]);
        return 1;
    }

    SenderConfig cfg;
    parse_sender_args(argc, argv, &cfg);

    if (!cfg.input_wav[0]) {
        fprintf(stderr, "请指定输入WAV文件\n");
        return 1;
    }

    /* 打开输入WAV */
    WAVFile wf;
    if (wav_open_read(&wf, cfg.input_wav) < 0) return 1;

    int sr = (int)wf.hdr.sample_rate;
    int ch = (int)wf.hdr.num_channels;
    int frame_ms = cfg.enc_cfg.frame_size_ms;
    int frame_samples = sr * frame_ms / 1000;
    int pcm_buf_size  = frame_samples * ch;

    cfg.enc_cfg.sample_rate = sr;
    cfg.enc_cfg.channels    = ch;

    printf("[发送端] %s → %s:%d\n", cfg.input_wav, cfg.host, cfg.port);
    printf("[发送端] %d Hz, %d ch, 帧长 %dms, 码率 %d bps\n",
           sr, ch, frame_ms, cfg.enc_cfg.bitrate);
    netsim_print_params(&cfg.net_cfg);

    /* 创建编码器 */
    int err;
    OpusEncoder *enc = opus_encoder_create(sr, ch,
                           cfg.enc_cfg.application, &err);
    if (!enc) { fprintf(stderr, "创建编码器失败\n"); return 1; }
    opus_encoder_ctl(enc, OPUS_SET_BITRATE(cfg.enc_cfg.bitrate));
    opus_encoder_ctl(enc, OPUS_SET_COMPLEXITY(cfg.enc_cfg.complexity));
    opus_encoder_ctl(enc, OPUS_SET_INBAND_FEC(cfg.enc_cfg.use_fec));
    opus_encoder_ctl(enc, OPUS_SET_PACKET_LOSS_PERC(
                              cfg.enc_cfg.packet_loss_perc));
    opus_encoder_ctl(enc, OPUS_SET_DTX(cfg.enc_cfg.use_dtx));
    opus_encoder_ctl(enc, OPUS_SET_VBR(cfg.enc_cfg.use_vbr));
    if (cfg.enc_cfg.dred_duration > 0)
        opus_encoder_ctl(enc, OPUS_SET_DRED_DURATION(
                                  cfg.enc_cfg.dred_duration));

    /* 加载DNN权重 */
    void *blob_data = NULL; int blob_len = 0;
    if (cfg.enc_cfg.dred_duration > 0) {
        blob_data = load_blob(DRED_BLOB_FILE, &blob_len);
        if (blob_data)
            opus_encoder_ctl(enc, OPUS_SET_DNN_BLOB(blob_data, blob_len));
    }

    /* 创建UDP socket */
    int sock = socket(AF_INET, SOCK_DGRAM, 0);
    if (sock < 0) { perror("socket"); return 1; }

    struct sockaddr_in dst;
    memset(&dst, 0, sizeof(dst));
    dst.sin_family = AF_INET;
    dst.sin_port   = htons((uint16_t)cfg.port);
    inet_pton(AF_INET, cfg.host, &dst.sin_addr);

    /* 初始化网络仿真 */
    NetSim ns;
    netsim_init(&ns, &cfg.net_cfg);
    netsim_seed(&ns, (uint64_t)time(NULL));

    /* 分配缓冲区 */
    int16_t *pcm_in    = (int16_t *)calloc(pcm_buf_size, sizeof(int16_t));
    uint8_t *send_buf  = (uint8_t *)malloc(MAX_PACKET_SIZE);
    uint8_t *opus_buf  = send_buf + RTP_HEADER_SIZE;

    uint16_t seq       = 0;
    uint32_t ts        = 0;
    uint32_t ssrc      = (uint32_t)time(NULL);
    int      sent      = 0, dropped = 0;
    int64_t  t_start   = now_us();

    printf("[发送端] 开始发送（Ctrl+C 停止）...\n");

    while (1) {
        /* 确定下一帧应该在何时发出（实时节奏）*/
        int64_t t_expected = t_start +
            (int64_t)((double)(sent + dropped) * frame_ms * 1000 / cfg.speed);
        int64_t t_now = now_us();
        long sleep_us = (long)(t_expected - t_now);
        if (sleep_us > 100) usleep_accurate(sleep_us);

        /* 读取音频帧 */
        int nread = wav_read_samples(&wf, pcm_in, frame_samples);
        if (nread <= 0) {
            /* 循环播放 */
            fseek(wf.fp, wf.data_offset, SEEK_SET);
            nread = wav_read_samples(&wf, pcm_in, frame_samples);
            if (nread <= 0) break;
        }
        if (nread < frame_samples)
            memset(pcm_in + nread * ch, 0,
                   (frame_samples - nread) * ch * sizeof(int16_t));

        /* 编码 */
        int nbytes = opus_encode(enc, pcm_in, frame_samples,
                                 opus_buf, MAX_PACKET_SIZE - RTP_HEADER_SIZE);
        if (nbytes < 0) {
            fprintf(stderr, "编码错误: %s\n", opus_strerror(nbytes));
            continue;
        }

        /* 网络仿真：决定是否丢包 */
        if (netsim_is_lost(&ns)) {
            dropped++;
            if (cfg.verbose)
                printf("[TX] seq=%5u 丢弃 (%.0f ms)\n", seq, netsim_delay_ms(&ns));
            seq++;
            ts += (uint32_t)frame_samples;
            continue;
        }

        /* 仿真时延（简单方式：sleep）*/
        double delay_ms = netsim_delay_ms(&ns);
        if (delay_ms > 1.0)
            usleep_accurate((long)(delay_ms * 1000));

        /* 填充RTP头 */
        RTPHeader *rtp = (RTPHeader *)send_buf;
        rtp_fill(rtp, seq, ts, ssrc);

        /* 发送 */
        ssize_t sret = sendto(sock, send_buf, RTP_HEADER_SIZE + nbytes,
                               0, (struct sockaddr *)&dst, sizeof(dst));
        if (sret < 0) {
            perror("sendto");
            break;
        }

        if (cfg.verbose)
            printf("[TX] seq=%5u size=%4d ts=%8u delay=%.1fms\n",
                   seq, nbytes, ts, delay_ms);

        seq++;
        ts   += (uint32_t)frame_samples;
        sent++;
    }

    printf("[发送端] 完成: 发送=%d 丢弃=%d 实际丢包率=%.1f%%\n",
           sent, dropped,
           (sent + dropped) > 0 ?
           100.0f * dropped / (sent + dropped) : 0.0f);

    wav_close_read(&wf);
    opus_encoder_destroy(enc);
    if (blob_data) free(blob_data);
    free(pcm_in);
    free(send_buf);
    close(sock);
    return 0;
}
