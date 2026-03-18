/**
 * common.h - Opus实验框架公共定义
 *
 * 包含：RTP包头结构、编解码器配置、网络仿真配置、统计结构
 */
#pragma once

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ========== 常量定义 ========== */
#define MAX_PACKET_SIZE      8192
#define MAX_FRAME_SIZE       5760    /* 48kHz * 120ms */
#define DEFAULT_SAMPLE_RATE  48000
#define DEFAULT_CHANNELS     1
#define RTP_HEADER_SIZE      12
#define OPUS_PAYLOAD_TYPE    111
#define DEFAULT_PORT         5004
#define DRED_BLOB_FILE       "weights_blob.bin"

/* ========== RTP 包头（简化版，12字节对齐） ========== */
#pragma pack(push, 1)
typedef struct {
    uint8_t  vpxcc;       /* version=2, padding=0, extension=0, csrc_count=0 → 0x80 */
    uint8_t  mpt;         /* marker=0, payload_type=111 → 0x6F */
    uint16_t seq;         /* 序列号 (网络字节序) */
    uint32_t timestamp;   /* RTP时间戳 (网络字节序) */
    uint32_t ssrc;        /* 同步源标识 */
} RTPHeader;
#pragma pack(pop)

/* 封装 RTP + Opus payload 的完整包 */
typedef struct {
    RTPHeader hdr;
    uint8_t   payload[MAX_PACKET_SIZE];
    int       payload_len;
} RTPPacket;

/* ========== Opus 编码器配置 ========== */
typedef struct {
    int sample_rate;       /* 采样率: 8000/16000/24000/48000 Hz */
    int channels;          /* 声道数: 1(单声道) 或 2(立体声) */
    int application;       /* OPUS_APPLICATION_VOIP / _AUDIO / _RESTRICTED_LOWDELAY */
    int bitrate;           /* 编码码率 (bps), -1=自动 */
    int frame_size_ms;     /* 帧长: 2.5/5/10/20/40/60 ms */
    int use_fec;           /* LBRR带内FEC: 0=关, 1=开 */
    int packet_loss_perc;  /* FEC优化用的预期丢包率 (0-100) */
    int use_dtx;           /* 不连续发送 DTX: 0=关, 1=开 */
    int use_vbr;           /* 可变码率 VBR: 0=CBR, 1=VBR */
    int dred_duration;     /* DRED冗余帧数 (单位: 10ms帧), 0=禁用 */
    int complexity;        /* 编码复杂度 0-10 */
    int signal_type;       /* OPUS_AUTO / OPUS_SIGNAL_VOICE / OPUS_SIGNAL_MUSIC */
    int lsb_depth;         /* 输入音频有效位深, 通常16 */
} EncoderConfig;

/* ========== Opus 解码器配置 ========== */
typedef struct {
    int sample_rate;
    int channels;
    int use_dred;          /* 是否使用DRED进行丢包恢复 */
    int use_lbrr;          /* 是否使用LBRR(FEC)进行丢包恢复 */
    int use_plc;           /* 是否使用PLC进行丢包隐藏 */
} DecoderConfig;

/* ========== 网络仿真配置 ========== */
typedef enum {
    LOSS_UNIFORM  = 0,    /* 均匀分布丢包 */
    LOSS_GILBERT  = 1,    /* Gilbert-Elliott 突发丢包模型 */
    LOSS_TRACE    = 2     /* 从文件读取丢包序列 */
} LossModel;

typedef struct {
    LossModel loss_model;

    /* 均匀丢包参数 */
    float loss_rate;           /* 平均丢包率 [0, 1] */

    /* Gilbert-Elliott 参数 */
    float p_good_to_bad;       /* GOOD→BAD 转移概率 */
    float p_bad_to_good;       /* BAD→GOOD 转移概率 */
    float loss_in_bad;         /* BAD状态下丢包率 [0,1] */
    float loss_in_good;        /* GOOD状态下丢包率 [0,1] */

    /* 时延/抖动参数 (毫秒) */
    float base_delay_ms;       /* 固定基础时延 */
    float jitter_std_ms;       /* 时延抖动标准差 (高斯分布) */
} NetSimConfig;

/* ========== 统计信息 ========== */
typedef struct {
    int   total_pkts;          /* 总发包数 */
    int   lost_pkts;           /* 丢包数 */
    int   recv_pkts;           /* 收包数 */
    int   recovered_lbrr;      /* 经LBRR恢复的帧数 */
    int   recovered_dred;      /* 经DRED恢复的帧数 */
    int   plc_frames;          /* PLC隐藏帧数 */
    int   dtx_frames;          /* DTX静音帧数 */
    long  total_encoded_bytes; /* 总编码字节数 */

    /* 每帧记录，供分析用 */
    int   *frame_status;       /* 0=收到, 1=LBRR恢复, 2=DRED恢复, 3=PLC */
    int   *frame_size_bytes;   /* 每帧编码字节数 */
    int    num_frames;
} SessionStats;

/* ========== 内联辅助函数 ========== */
static inline uint16_t hton16(uint16_t v) {
    return (uint16_t)((v >> 8) | (v << 8));
}
static inline uint32_t hton32(uint32_t v) {
    return ((v >> 24) & 0xff)       |
           ((v >> 8)  & 0xff00)     |
           ((v << 8)  & 0xff0000)   |
           ((v << 24) & 0xff000000);
}
static inline uint16_t ntoh16(uint16_t v) { return hton16(v); }
static inline uint32_t ntoh32(uint32_t v) { return hton32(v); }

static inline void rtp_fill(RTPHeader *h, uint16_t seq,
                             uint32_t ts, uint32_t ssrc) {
    h->vpxcc     = 0x80;
    h->mpt       = OPUS_PAYLOAD_TYPE & 0x7F;
    h->seq       = hton16(seq);
    h->timestamp = hton32(ts);
    h->ssrc      = hton32(ssrc);
}

/* 打印会话统计 */
static inline void stats_print(const SessionStats *s) {
    printf("\n========== 会话统计 ==========\n");
    printf("总帧数      : %d\n", s->total_pkts);
    printf("丢包数      : %d (%.1f%%)\n",
           s->lost_pkts, s->total_pkts > 0 ?
           100.0 * s->lost_pkts / s->total_pkts : 0.0);
    printf("收包数      : %d\n", s->recv_pkts);
    printf("LBRR恢复    : %d\n", s->recovered_lbrr);
    printf("DRED恢复    : %d\n", s->recovered_dred);
    printf("PLC隐藏     : %d\n", s->plc_frames);
    printf("DTX帧       : %d\n", s->dtx_frames);
    if (s->total_pkts > 0 && s->total_encoded_bytes > 0) {
        printf("平均包大小  : %.1f bytes\n",
               (double)s->total_encoded_bytes / s->total_pkts);
    }
    printf("================================\n\n");
}
