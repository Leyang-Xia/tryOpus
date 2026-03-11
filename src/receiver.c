/**
 * receiver.c - Opus UDP 接收端（含简单抖动缓冲）
 *
 * 接收 opus_sender 发出的RTP/Opus包，
 * 支持 PLC / LBRR / DRED 丢包恢复，
 * 将解码后的PCM写入WAV文件。
 *
 * 用法: opus_receiver [选项] output.wav
 *   -p <port>    监听端口 (默认: 5004)
 *   -r <hz>      采样率 (默认: 48000)
 *   -c <n>       声道数 (默认: 1)
 *   -fs <ms>     帧长 (默认: 20)
 *   -jbuf <ms>   抖动缓冲大小 (默认: 60ms)
 *   --no-dred    禁用DRED恢复
 *   --no-lbrr    禁用LBRR恢复
 *   --no-plc     禁用PLC（丢包填零）
 *   -t <sec>     录制时长 (默认: 30)
 *   -v           详细输出
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <time.h>
#include <unistd.h>
#include <signal.h>
#include <arpa/inet.h>
#include <sys/socket.h>
#include <sys/select.h>
#include <netinet/in.h>

#include "common.h"
#include "wav_io.h"
#include <opus/opus.h>

static volatile int g_running = 1;
static void sig_handler(int s) { (void)s; g_running = 0; }

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

/* ========== 简单抖动缓冲（环形，按序列号排序） ========== */
#define JBUF_SIZE 64   /* 必须是2的幂 */

typedef struct {
    uint8_t  data[MAX_PACKET_SIZE];
    int      len;
    uint16_t seq;
    int      valid;
} JBufSlot;

typedef struct {
    JBufSlot slots[JBUF_SIZE];
    uint16_t next_seq;      /* 下一个期望接收的序列号 */
    int      initialized;
    int      target_depth;  /* 目标缓冲深度（帧数）*/
    int      fill_count;    /* 当前缓冲帧数 */
} JitterBuffer;

static void jbuf_init(JitterBuffer *jb, int target_depth_frames) {
    memset(jb, 0, sizeof(*jb));
    jb->target_depth  = target_depth_frames;
    jb->initialized   = 0;
}

/* 将收到的包放入抖动缓冲 */
static void jbuf_push(JitterBuffer *jb, uint16_t seq,
                       const uint8_t *data, int len) {
    if (!jb->initialized) {
        jb->next_seq    = seq;
        jb->initialized = 1;
    }
    int idx = seq % JBUF_SIZE;
    jb->slots[idx].seq   = seq;
    jb->slots[idx].len   = len;
    jb->slots[idx].valid = 1;
    memcpy(jb->slots[idx].data, data,
           len < MAX_PACKET_SIZE ? len : MAX_PACKET_SIZE);
    jb->fill_count++;
}

/* 从抖动缓冲取出下一帧
 * 返回 1=有数据, 0=缓冲不足, -1=该序列号丢失 */
static int jbuf_pop(JitterBuffer *jb, uint8_t **out_data, int *out_len) {
    if (!jb->initialized) return 0;
    if (jb->fill_count < jb->target_depth) return 0;  /* 等待填充 */

    int idx = jb->next_seq % JBUF_SIZE;
    if (jb->slots[idx].valid && jb->slots[idx].seq == jb->next_seq) {
        *out_data = jb->slots[idx].data;
        *out_len  = jb->slots[idx].len;
        jb->slots[idx].valid = 0;
        jb->next_seq++;
        if (jb->fill_count > 0) jb->fill_count--;
        return 1;
    } else {
        /* 丢失 */
        *out_data = NULL;
        *out_len  = 0;
        jb->next_seq++;
        if (jb->fill_count > 0) jb->fill_count--;
        return -1;
    }
}

/* 窥视下一帧（LBRR用：需要看序列号seq+1的包） */
static JBufSlot *jbuf_peek(JitterBuffer *jb, uint16_t seq) {
    int idx = seq % JBUF_SIZE;
    if (jb->slots[idx].valid && jb->slots[idx].seq == seq)
        return &jb->slots[idx];
    return NULL;
}

/* ========== 接收端配置 ========== */
typedef struct {
    int     port;
    int     sample_rate;
    int     channels;
    int     frame_size_ms;
    int     jbuf_depth_ms;
    int     record_sec;
    int     use_dred;
    int     use_lbrr;
    int     use_plc;
    int     verbose;
    char    output_wav[256];
} ReceiverConfig;

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "用法: %s [选项] output.wav\n", argv[0]);
        return 1;
    }

    ReceiverConfig cfg;
    memset(&cfg, 0, sizeof(cfg));
    cfg.port           = DEFAULT_PORT;
    cfg.sample_rate    = DEFAULT_SAMPLE_RATE;
    cfg.channels       = DEFAULT_CHANNELS;
    cfg.frame_size_ms  = 20;
    cfg.jbuf_depth_ms  = 60;
    cfg.record_sec     = 30;
    cfg.use_dred       = 1;
    cfg.use_lbrr       = 1;
    cfg.use_plc        = 1;

    for (int i = 1; i < argc; i++) {
        if      (!strcmp(argv[i], "-p"))     cfg.port = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-r"))     cfg.sample_rate = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-c"))     cfg.channels = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-fs"))    cfg.frame_size_ms = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-jbuf"))  cfg.jbuf_depth_ms = atoi(argv[++i]);
        else if (!strcmp(argv[i], "-t"))     cfg.record_sec = atoi(argv[++i]);
        else if (!strcmp(argv[i], "--no-dred")) cfg.use_dred = 0;
        else if (!strcmp(argv[i], "--no-lbrr")) cfg.use_lbrr = 0;
        else if (!strcmp(argv[i], "--no-plc"))  cfg.use_plc = 0;
        else if (!strcmp(argv[i], "-v"))     cfg.verbose = 1;
        else if (argv[i][0] != '-')
            strncpy(cfg.output_wav, argv[i], sizeof(cfg.output_wav) - 1);
    }

    int sr = cfg.sample_rate, ch = cfg.channels;
    int frame_ms = cfg.frame_size_ms;
    int frame_samples = sr * frame_ms / 1000;
    int jbuf_depth_frames = cfg.jbuf_depth_ms / frame_ms;
    if (jbuf_depth_frames < 1) jbuf_depth_frames = 1;

    printf("[接收端] 监听端口 %d → %s\n", cfg.port, cfg.output_wav);
    printf("[接收端] %d Hz, %d ch, 帧长 %dms, 抖动缓冲 %dms\n",
           sr, ch, frame_ms, cfg.jbuf_depth_ms);

    /* 创建解码器 */
    int err;
    OpusDecoder *dec = opus_decoder_create(sr, ch, &err);
    if (!dec) { fprintf(stderr, "创建解码器失败\n"); return 1; }

    /* 加载DNN权重 */
    void *blob_data = NULL; int blob_len = 0;
    if (cfg.use_dred) {
        blob_data = load_blob(DRED_BLOB_FILE, &blob_len);
        if (blob_data)
            opus_decoder_ctl(dec, OPUS_SET_DNN_BLOB(blob_data, blob_len));
    }

    /* 创建DRED解码器 */
    OpusDREDDecoder *dred_dec = NULL;
    OpusDRED        *dred     = NULL;
    if (cfg.use_dred) {
        dred_dec = opus_dred_decoder_create(&err);
        dred     = opus_dred_alloc(&err);
        if (dred_dec && dred && blob_data)
            opus_dred_decoder_ctl(dred_dec, OPUS_SET_DNN_BLOB(blob_data, blob_len));
        printf("[接收端] DRED解码器: 已启用\n");
    }

    /* 创建UDP socket */
    int sock = socket(AF_INET, SOCK_DGRAM, 0);
    if (sock < 0) { perror("socket"); return 1; }
    int reuse = 1;
    setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, &reuse, sizeof(reuse));

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family      = AF_INET;
    addr.sin_port        = htons((uint16_t)cfg.port);
    addr.sin_addr.s_addr = INADDR_ANY;
    if (bind(sock, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        perror("bind"); return 1;
    }

    /* 设置超时（frame_ms * 2） */
    struct timeval timeout;
    timeout.tv_sec  = 0;
    timeout.tv_usec = frame_ms * 2 * 1000;
    setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout));

    /* 打开输出WAV */
    WAVFile wf_out;
    if (wav_open_write(&wf_out, cfg.output_wav, sr, ch) < 0) return 1;

    /* 初始化缓冲区 */
    JitterBuffer jb;
    jbuf_init(&jb, jbuf_depth_frames);

    int16_t *pcm_out  = (int16_t *)calloc(frame_samples * ch, sizeof(int16_t));
    uint8_t *recv_buf = (uint8_t *)malloc(MAX_PACKET_SIZE);

    int  total_recv = 0, total_lost = 0;
    int  lbrr_cnt = 0, dred_cnt = 0, plc_cnt = 0;
    time_t t_start = time(NULL);
    int  consecutive_lost = 0;

    signal(SIGINT,  sig_handler);
    signal(SIGTERM, sig_handler);

    printf("[接收端] 等待数据（录制%d秒，Ctrl+C停止）...\n", cfg.record_sec);

    while (g_running && (time(NULL) - t_start) < cfg.record_sec) {
        /* 接收UDP包 */
        struct sockaddr_in src;
        socklen_t src_len = sizeof(src);
        ssize_t rlen = recvfrom(sock, recv_buf, MAX_PACKET_SIZE, 0,
                                 (struct sockaddr *)&src, &src_len);

        if (rlen > (ssize_t)RTP_HEADER_SIZE) {
            RTPHeader *rtp = (RTPHeader *)recv_buf;
            uint16_t seq   = ntoh16(rtp->seq);
            uint8_t  *opus = recv_buf + RTP_HEADER_SIZE;
            int       olen = (int)(rlen - RTP_HEADER_SIZE);

            jbuf_push(&jb, seq, opus, olen);
            total_recv++;
        }

        /* 尝试从抖动缓冲取帧 */
        uint8_t *pkt_data = NULL;
        int      pkt_len  = 0;
        int      pop_ret  = jbuf_pop(&jb, &pkt_data, &pkt_len);

        if (pop_ret == 0) continue;  /* 缓冲未满 */

        int decode_ret;
        const char *status_str;

        if (pop_ret == 1 && pkt_data && pkt_len > 1) {
            /* 正常解码 */
            consecutive_lost = 0;
            decode_ret = opus_decode(dec, pkt_data, pkt_len,
                                      pcm_out, frame_samples, 0);
            status_str = "OK";
        } else {
            /* 丢包 */
            total_lost++;
            consecutive_lost++;
            int recovered = 0;

            /* LBRR: 尝试从下一包的FEC数据恢复 */
            if (!recovered && cfg.use_lbrr) {
                uint16_t next_seq = jb.next_seq;  /* 当前等待的下一帧 */
                JBufSlot *next_slot = jbuf_peek(&jb, next_seq);
                if (next_slot && next_slot->len > 1) {
                    decode_ret = opus_decode(dec, next_slot->data,
                                              next_slot->len,
                                              pcm_out, frame_samples, 1);
                    if (decode_ret > 0) {
                        recovered = 1;
                        lbrr_cnt++;
                        status_str = "LBRR";
                    }
                }
            }

            /* DRED: 从下一个到达包的DRED payload恢复 */
            if (!recovered && cfg.use_dred && dred_dec && dred) {
                uint16_t next_seq = jb.next_seq;
                JBufSlot *next_slot = jbuf_peek(&jb, next_seq);
                if (next_slot && next_slot->len > 1) {
                    int dred_end = 0;
                    int ret = opus_dred_parse(dred_dec, dred,
                                              next_slot->data, next_slot->len,
                                              consecutive_lost * frame_samples,
                                              sr, &dred_end, 0);
                    if (ret > 0) {
                        decode_ret = opus_decoder_dred_decode(
                            dec, dred,
                            (opus_int32)((consecutive_lost - 1) * frame_samples),
                            pcm_out, frame_samples);
                        if (decode_ret > 0) {
                            recovered = 1;
                            dred_cnt++;
                            status_str = "DRED";
                        }
                    }
                }
            }

            /* PLC */
            if (!recovered && cfg.use_plc) {
                decode_ret = opus_decode(dec, NULL, 0,
                                          pcm_out, frame_samples, 0);
                plc_cnt++;
                status_str = "PLC";
            } else if (!recovered) {
                memset(pcm_out, 0, frame_samples * ch * sizeof(int16_t));
                decode_ret = frame_samples;
                status_str = "MUTE";
            }
        }

        if (decode_ret > 0)
            wav_write_samples(&wf_out, pcm_out, decode_ret);

        if (cfg.verbose)
            printf("[RX] seq=%5u %s size=%d\n",
                   (unsigned)(jb.next_seq - 1), status_str, pkt_len);
    }

    /* 统计输出 */
    printf("\n[接收端] 统计:\n");
    printf("  收包: %d  丢包: %d (%.1f%%)\n",
           total_recv, total_lost,
           (total_recv + total_lost) > 0 ?
           100.0f * total_lost / (total_recv + total_lost) : 0.0f);
    printf("  LBRR恢复: %d  DRED恢复: %d  PLC隐藏: %d\n",
           lbrr_cnt, dred_cnt, plc_cnt);

    wav_close_write(&wf_out);
    close(sock);
    opus_decoder_destroy(dec);
    if (dred)     opus_dred_free(dred);
    if (dred_dec) opus_dred_decoder_destroy(dred_dec);
    if (blob_data) free(blob_data);
    free(pcm_out);
    free(recv_buf);
    return 0;
}
