/**
 * opus_sim.c - Opus 编解码离线仿真工具
 *
 * 功能：
 *   1. 读取WAV → Opus编码 → 信道模拟(丢包/抖动) → Opus解码 → 输出WAV
 *   2. 支持 LBRR (带内FEC)、DRED (深度冗余)、PLC 丢包隐藏
 *   3. 支持 DTX（不连续发送）
 *   4. 输出每帧统计信息和总体报告
 *
 * 用法: 见 --help
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <time.h>

#include "common.h"
#include "wav_io.h"
#include "netsim.h"

/* 引入 Opus 头文件 */
#include <opus/opus.h>

/* ========== DNN blob 加载 ========== */
static void *load_blob(const char *path, int *out_len) {
    FILE *f = fopen(path, "rb");
    if (!f) return NULL;
    fseek(f, 0, SEEK_END);
    long len = ftell(f);
    fseek(f, 0, SEEK_SET);
    void *buf = malloc((size_t)len);
    if (!buf) { fclose(f); return NULL; }
    if (fread(buf, 1, (size_t)len, f) != (size_t)len) {
        free(buf); fclose(f); return NULL;
    }
    fclose(f);
    *out_len = (int)len;
    return buf;
}

/* ========== 帧状态枚举 ========== */
typedef enum {
    FRAME_OK    = 0,
    FRAME_LBRR  = 1,  /* 通过LBRR/FEC恢复 */
    FRAME_DRED  = 2,  /* 通过DRED恢复 */
    FRAME_PLC   = 3,  /* PLC隐藏 */
    FRAME_LOST  = 4   /* 完全丢失（DTX或编码失败）*/
} FrameStatus;

/* ========== 命令行参数 ========== */
typedef struct {
    char input_wav[256];
    char output_wav[256];
    char stats_csv[256];  /* 可选：输出统计CSV */

    /* 编码器 */
    EncoderConfig enc_cfg;

    /* 解码器 */
    DecoderConfig dec_cfg;

    /* 网络仿真 */
    NetSimConfig  net_cfg;

    /* 其他 */
    int  verbose;          /* 打印每帧详情 */
    int  no_audio_output;  /* 仅统计不写音频 */
    int  dred_lookahead;   /* DRED超前帧数（接收到未来帧后恢复丢包） */
} SimConfig;

static void print_usage(const char *prog) {
    printf(
        "用法: %s [选项] 输入.wav 输出.wav\n"
        "\n"
        "编码器选项:\n"
        "  -b,  --bitrate   <bps>     码率 (默认: 32000)\n"
        "  -fs, --framesize <ms>      帧长 2.5/5/10/20/40/60 (默认: 20)\n"
        "  -fec,--lbrr                开启LBRR带内FEC\n"
        "  -plp,--ploss    <pct>      向编码器声明的丢包率(影响FEC强度, 0-100)\n"
        "  -dtx,--dtx                 开启DTX\n"
        "  -vbr,--vbr                 开启VBR\n"
        "  -dred <n>                  DRED冗余帧数 (单位:10ms, 推荐2-10)\n"
        "  -cx, --complexity <0-10>   编码复杂度 (默认:9)\n"
        "  -app <voip|audio|ll>       应用类型 (默认:voip)\n"
        "\n"
        "解码器选项:\n"
        "  --no-dred                  禁用DRED恢复\n"
        "  --no-lbrr                  禁用LBRR恢复\n"
        "  --no-plc                   禁用PLC（丢包输出静音）\n"
        "\n"
        "网络仿真选项:\n"
        "  -l,  --loss      <rate>    均匀丢包率 [0,1] (默认:0)\n"
        "  -ge                        使用Gilbert-Elliott模型\n"
        "  -ge-p2b <p>                GE: GOOD→BAD转移概率 (默认:0.05)\n"
        "  -ge-b2g <p>                GE: BAD→GOOD转移概率 (默认:0.3)\n"
        "  -ge-bloss <p>              GE: BAD状态丢包率 (默认:0.8)\n"
        "  -d,  --delay     <ms>      固定时延 (默认:0)\n"
        "  -j,  --jitter    <ms>      时延抖动标准差 (默认:0)\n"
        "\n"
        "输出选项:\n"
        "  -v,  --verbose             打印每帧详情\n"
        "  --csv <file>               输出统计CSV文件\n"
        "\n"
        "示例:\n"
        "  # 基础测试: 10%%丢包 + LBRR\n"
        "  %s -fec -plp 10 -l 0.1 input.wav output.wav\n"
        "\n"
        "  # DRED测试: 突发丢包 + DRED 3帧冗余\n"
        "  %s -dred 3 -ge -ge-p2b 0.05 -ge-b2g 0.3 input.wav output.wav\n"
        "\n"
        "  # 组合: LBRR+DRED双重保护\n"
        "  %s -fec -plp 15 -dred 5 -l 0.1 -j 20 input.wav output.wav\n",
        prog, prog, prog, prog
    );
}

/* ========== 参数解析 ========== */
static int parse_args(int argc, char **argv, SimConfig *cfg) {
    memset(cfg, 0, sizeof(*cfg));

    /* 编码器默认值 */
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
    cfg->enc_cfg.lsb_depth        = 16;

    /* 解码器默认值 */
    cfg->dec_cfg.sample_rate = DEFAULT_SAMPLE_RATE;
    cfg->dec_cfg.channels    = DEFAULT_CHANNELS;
    cfg->dec_cfg.use_dred    = 1;
    cfg->dec_cfg.use_lbrr    = 1;
    cfg->dec_cfg.use_plc     = 1;

    /* 网络仿真默认值 */
    cfg->net_cfg.loss_model    = LOSS_UNIFORM;
    cfg->net_cfg.loss_rate     = 0.0f;
    cfg->net_cfg.p_good_to_bad = 0.05f;
    cfg->net_cfg.p_bad_to_good = 0.30f;
    cfg->net_cfg.loss_in_bad   = 0.80f;
    cfg->net_cfg.loss_in_good  = 0.02f;
    cfg->net_cfg.base_delay_ms = 0.0f;
    cfg->net_cfg.jitter_std_ms = 0.0f;

    cfg->dred_lookahead = 1;  /* 等1帧后恢复DRED */

    int i;
    for (i = 1; i < argc; i++) {
        if (!strcmp(argv[i], "-h") || !strcmp(argv[i], "--help")) {
            return -1;
        } else if (!strcmp(argv[i], "-b") || !strcmp(argv[i], "--bitrate")) {
            cfg->enc_cfg.bitrate = atoi(argv[++i]);
        } else if (!strcmp(argv[i], "-fs") || !strcmp(argv[i], "--framesize")) {
            cfg->enc_cfg.frame_size_ms = atoi(argv[++i]);
        } else if (!strcmp(argv[i], "-fec") || !strcmp(argv[i], "--lbrr")) {
            cfg->enc_cfg.use_fec = 1;
        } else if (!strcmp(argv[i], "-plp") || !strcmp(argv[i], "--ploss")) {
            cfg->enc_cfg.packet_loss_perc = atoi(argv[++i]);
        } else if (!strcmp(argv[i], "-dtx") || !strcmp(argv[i], "--dtx")) {
            cfg->enc_cfg.use_dtx = 1;
        } else if (!strcmp(argv[i], "-vbr") || !strcmp(argv[i], "--vbr")) {
            cfg->enc_cfg.use_vbr = 1;
        } else if (!strcmp(argv[i], "-dred")) {
            cfg->enc_cfg.dred_duration = atoi(argv[++i]);
        } else if (!strcmp(argv[i], "-cx") || !strcmp(argv[i], "--complexity")) {
            cfg->enc_cfg.complexity = atoi(argv[++i]);
        } else if (!strcmp(argv[i], "-app")) {
            const char *a = argv[++i];
            if      (!strcmp(a, "voip"))  cfg->enc_cfg.application = OPUS_APPLICATION_VOIP;
            else if (!strcmp(a, "audio")) cfg->enc_cfg.application = OPUS_APPLICATION_AUDIO;
            else if (!strcmp(a, "ll"))    cfg->enc_cfg.application = OPUS_APPLICATION_RESTRICTED_LOWDELAY;
        } else if (!strcmp(argv[i], "--no-dred")) {
            cfg->dec_cfg.use_dred = 0;
        } else if (!strcmp(argv[i], "--no-lbrr")) {
            cfg->dec_cfg.use_lbrr = 0;
        } else if (!strcmp(argv[i], "--no-plc")) {
            cfg->dec_cfg.use_plc = 0;
        } else if (!strcmp(argv[i], "-l") || !strcmp(argv[i], "--loss")) {
            cfg->net_cfg.loss_rate = (float)atof(argv[++i]);
        } else if (!strcmp(argv[i], "-ge")) {
            cfg->net_cfg.loss_model = LOSS_GILBERT;
        } else if (!strcmp(argv[i], "-ge-p2b")) {
            cfg->net_cfg.p_good_to_bad = (float)atof(argv[++i]);
        } else if (!strcmp(argv[i], "-ge-b2g")) {
            cfg->net_cfg.p_bad_to_good = (float)atof(argv[++i]);
        } else if (!strcmp(argv[i], "-ge-bloss")) {
            cfg->net_cfg.loss_in_bad = (float)atof(argv[++i]);
        } else if (!strcmp(argv[i], "-d") || !strcmp(argv[i], "--delay")) {
            cfg->net_cfg.base_delay_ms = (float)atof(argv[++i]);
        } else if (!strcmp(argv[i], "-j") || !strcmp(argv[i], "--jitter")) {
            cfg->net_cfg.jitter_std_ms = (float)atof(argv[++i]);
        } else if (!strcmp(argv[i], "-v") || !strcmp(argv[i], "--verbose")) {
            cfg->verbose = 1;
        } else if (!strcmp(argv[i], "--csv")) {
            strncpy(cfg->stats_csv, argv[++i], sizeof(cfg->stats_csv) - 1);
        } else if (argv[i][0] != '-') {
            /* 位置参数: 输入/输出文件 */
            if (!cfg->input_wav[0])
                strncpy(cfg->input_wav, argv[i], sizeof(cfg->input_wav) - 1);
            else
                strncpy(cfg->output_wav, argv[i], sizeof(cfg->output_wav) - 1);
        } else {
            fprintf(stderr, "未知选项: %s\n", argv[i]);
            return -1;
        }
    }

    /* 网络仿真: 如果指定了GE模式但没有设置loss_rate，
       则由GE参数自动计算均值 */
    if (cfg->net_cfg.loss_model == LOSS_GILBERT &&
        cfg->net_cfg.loss_rate > 0.0f) {
        /* loss_rate作为均值，调整p_good_to_bad */
        /* mean_loss = pi_bad * loss_bad + pi_good * loss_good
           pi_bad = p_g2b / (p_g2b + p_b2g) */
        /* 如果用户指定了loss_rate，覆盖loss_in_bad */
        /* 简单起见: 直接用loss_rate作为loss_in_bad */
        cfg->net_cfg.loss_in_bad = cfg->net_cfg.loss_rate < 0.9f ?
                                   cfg->net_cfg.loss_rate * 4.0f : 0.9f;
        if (cfg->net_cfg.loss_in_bad > 0.95f) cfg->net_cfg.loss_in_bad = 0.95f;
    }

    if (!cfg->input_wav[0] || !cfg->output_wav[0]) {
        fprintf(stderr, "需要指定输入和输出WAV文件\n");
        return -1;
    }
    return 0;
}

/* ========== 主仿真循环 ========== */
int main(int argc, char **argv) {
    SimConfig cfg;
    if (parse_args(argc, argv, &cfg) < 0) {
        print_usage(argv[0]);
        return 1;
    }

    /* ---- 打印配置 ---- */
    printf("=== Opus 仿真工具 ===\n");
    printf("输入文件    : %s\n", cfg.input_wav);
    printf("输出文件    : %s\n", cfg.output_wav);
    printf("码率        : %d bps\n", cfg.enc_cfg.bitrate);
    printf("帧长        : %d ms\n", cfg.enc_cfg.frame_size_ms);
    printf("LBRR/FEC    : %s\n", cfg.enc_cfg.use_fec ? "开" : "关");
    printf("DRED冗余    : %d 帧 (10ms)\n", cfg.enc_cfg.dred_duration);
    printf("DTX         : %s\n", cfg.enc_cfg.use_dtx ? "开" : "关");
    printf("VBR         : %s\n", cfg.enc_cfg.use_vbr ? "开" : "关");
    printf("复杂度      : %d\n", cfg.enc_cfg.complexity);
    netsim_print_params(&cfg.net_cfg);

    /* ---- 打开输入WAV ---- */
    WAVFile wf_in;
    if (wav_open_read(&wf_in, cfg.input_wav) < 0) return 1;
    printf("输入WAV     : %d Hz, %d ch, %d 样本\n",
           wf_in.hdr.sample_rate, wf_in.hdr.num_channels, wf_in.num_samples);

    /* 根据WAV调整编解码器配置 */
    cfg.enc_cfg.sample_rate  = (int)wf_in.hdr.sample_rate;
    cfg.enc_cfg.channels     = (int)wf_in.hdr.num_channels;
    cfg.dec_cfg.sample_rate  = cfg.enc_cfg.sample_rate;
    cfg.dec_cfg.channels     = cfg.enc_cfg.channels;

    int sr       = cfg.enc_cfg.sample_rate;
    int ch       = cfg.enc_cfg.channels;
    int frame_ms = cfg.enc_cfg.frame_size_ms;
    /* 每帧样本数 = 采样率 * 帧时长 / 1000 */
    int frame_samples = sr * frame_ms / 1000;  /* 每声道 */

    printf("帧样本数    : %d (每声道)\n\n", frame_samples);

    /* ---- 创建编码器 ---- */
    int err = 0;
    OpusEncoder *enc = opus_encoder_create(sr, ch,
                           cfg.enc_cfg.application, &err);
    if (err != OPUS_OK || !enc) {
        fprintf(stderr, "创建编码器失败: %s\n", opus_strerror(err));
        return 1;
    }
    opus_encoder_ctl(enc, OPUS_SET_BITRATE(cfg.enc_cfg.bitrate));
    opus_encoder_ctl(enc, OPUS_SET_COMPLEXITY(cfg.enc_cfg.complexity));
    opus_encoder_ctl(enc, OPUS_SET_INBAND_FEC(cfg.enc_cfg.use_fec));
    opus_encoder_ctl(enc, OPUS_SET_DTX(cfg.enc_cfg.use_dtx));
    opus_encoder_ctl(enc, OPUS_SET_VBR(cfg.enc_cfg.use_vbr));
    opus_encoder_ctl(enc, OPUS_SET_LSB_DEPTH(cfg.enc_cfg.lsb_depth));
    if (cfg.enc_cfg.dred_duration > 0) {
        opus_encoder_ctl(enc, OPUS_SET_DRED_DURATION(
                                  cfg.enc_cfg.dred_duration));
        /* 关键：DRED 编码量由 packet_loss_perc 驱动，
           若用户未指定，则根据仿真丢包率自动推算 */
        if (cfg.enc_cfg.packet_loss_perc == 0) {
            int auto_plp = (int)(cfg.net_cfg.loss_rate * 100 + 0.5f);
            if (auto_plp < 5) auto_plp = 10;  /* 至少给10%让DRED有冗余预算 */
            cfg.enc_cfg.packet_loss_perc = auto_plp;
        }
    }
    opus_encoder_ctl(enc, OPUS_SET_PACKET_LOSS_PERC(
                              cfg.enc_cfg.packet_loss_perc));

    /* ---- 加载 DNN weights（DRED/DeepPLC 需要） ---- */
    void *blob_data = NULL;
    int   blob_len  = 0;
    if (cfg.enc_cfg.dred_duration > 0 || cfg.dec_cfg.use_dred) {
        blob_data = load_blob(DRED_BLOB_FILE, &blob_len);
        if (!blob_data) {
            fprintf(stderr, "警告: 找不到 %s，DRED 将被禁用\n",
                    DRED_BLOB_FILE);
            cfg.enc_cfg.dred_duration = 0;
            cfg.dec_cfg.use_dred      = 0;
        } else {
            printf("已加载DNN权重: %d bytes\n", blob_len);
            opus_encoder_ctl(enc, OPUS_SET_DNN_BLOB(blob_data, blob_len));
        }
    }

    /* ---- 创建解码器 ---- */
    OpusDecoder *dec = opus_decoder_create(sr, ch, &err);
    if (err != OPUS_OK || !dec) {
        fprintf(stderr, "创建解码器失败: %s\n", opus_strerror(err));
        return 1;
    }
    if (blob_data)
        opus_decoder_ctl(dec, OPUS_SET_DNN_BLOB(blob_data, blob_len));

    /* ---- 创建 DRED 解码器 ---- */
    OpusDREDDecoder *dred_dec = NULL;
    OpusDRED        *dred     = NULL;
    if (cfg.dec_cfg.use_dred && cfg.enc_cfg.dred_duration > 0) {
        dred_dec = opus_dred_decoder_create(&err);
        if (err != OPUS_OK) {
            fprintf(stderr, "创建DRED解码器失败: %s\n", opus_strerror(err));
            cfg.dec_cfg.use_dred = 0;
        } else {
            dred = opus_dred_alloc(&err);
            if (err != OPUS_OK) {
                fprintf(stderr, "分配DRED状态失败\n");
                cfg.dec_cfg.use_dred = 0;
            }
            if (blob_data && dred_dec)
                opus_dred_decoder_ctl(dred_dec,
                                      OPUS_SET_DNN_BLOB(blob_data, blob_len));
        }
        if (cfg.dec_cfg.use_dred)
            printf("DRED解码器   : 已启用\n");
    }

    /* ---- 打开输出WAV ---- */
    WAVFile wf_out;
    if (!cfg.no_audio_output) {
        if (wav_open_write(&wf_out, cfg.output_wav, sr, ch) < 0) return 1;
    }

    /* ---- 分配缓冲区 ---- */
    int   pcm_buf_size = frame_samples * ch;
    int16_t *pcm_in  = (int16_t *)calloc(pcm_buf_size, sizeof(int16_t));
    int16_t *pcm_out = (int16_t *)calloc(pcm_buf_size, sizeof(int16_t));
    uint8_t *enc_buf = (uint8_t *)malloc(MAX_PACKET_SIZE);

    /* ---- 预编码所有帧（离线仿真需要超前访问用于DRED/LBRR） ---- */
    int max_frames = (wf_in.num_samples + frame_samples - 1) / frame_samples + 1;
    uint8_t **enc_pkts  = (uint8_t **)calloc(max_frames, sizeof(uint8_t *));
    int      *enc_sizes = (int      *)calloc(max_frames, sizeof(int));
    int       num_frames = 0;

    printf("正在编码...\n");
    while (1) {
        int nread = wav_read_samples(&wf_in, pcm_in, frame_samples);
        if (nread <= 0) break;
        /* 不足一帧补零 */
        if (nread < frame_samples)
            memset(pcm_in + nread * ch, 0,
                   (frame_samples - nread) * ch * sizeof(int16_t));

        int nbytes = opus_encode(enc, pcm_in, frame_samples,
                                 enc_buf, MAX_PACKET_SIZE);
        if (nbytes < 0) {
            fprintf(stderr, "编码错误 frame=%d: %s\n",
                    num_frames, opus_strerror(nbytes));
            nbytes = 0;
        }

        enc_pkts[num_frames]  = (uint8_t *)malloc(nbytes > 0 ? nbytes : 1);
        enc_sizes[num_frames] = nbytes;
        if (nbytes > 0)
            memcpy(enc_pkts[num_frames], enc_buf, nbytes);
        num_frames++;
        if (num_frames >= max_frames) break;
    }
    wav_close_read(&wf_in);
    printf("编码完成: %d 帧\n\n", num_frames);

    /* ---- 初始化网络仿真 ---- */
    NetSim ns;
    netsim_init(&ns, &cfg.net_cfg);
    netsim_seed(&ns, (uint64_t)time(NULL));

    /* ---- 仿真丢包模式（离线，提前确定） ---- */
    int *is_lost = (int *)calloc(num_frames, sizeof(int));
    for (int i = 0; i < num_frames; i++)
        is_lost[i] = netsim_is_lost(&ns);

    /* ---- 统计初始化 ---- */
    SessionStats stats;
    memset(&stats, 0, sizeof(stats));
    stats.total_pkts    = num_frames;
    stats.frame_status  = (int *)calloc(num_frames, sizeof(int));
    stats.frame_size_bytes = (int *)calloc(num_frames, sizeof(int));
    stats.num_frames    = num_frames;

    FILE *csv_fp = NULL;
    if (cfg.stats_csv[0]) {
        csv_fp = fopen(cfg.stats_csv, "w");
        if (csv_fp)
            fprintf(csv_fp, "frame,size_bytes,lost,status\n");
    }

    /* ---- 解码循环（突发感知）----
     *
     * 处理逻辑：
     *   - 非丢包帧：直接解码
     *   - 丢包突发 [burst_start..burst_end]，下一个到达包为 next_ok：
     *       - LBRR: 仅恢复 burst_end（突发中最近丢失帧），从 next_ok 包的FEC数据
     *       - DRED: 从 next_ok 包中解析DRED，按序（burst_start→burst_end）恢复
     *              偏移量 = (next_ok - i) * frame_samples（i=当前丢包帧索引）
     *       - PLC:  以上失败时回退到PLC
     */
    printf("正在解码...\n");
    if (cfg.verbose) {
        printf("%-6s %-8s %-6s %-10s\n",
               "帧号", "大小(B)", "丢失", "恢复方式");
        printf("------------------------------------\n");
    }

    int i = 0;
    while (i < num_frames) {
        int pkt_size   = enc_sizes[i];
        stats.frame_size_bytes[i] = pkt_size;
        stats.total_encoded_bytes += pkt_size;

        FrameStatus fstatus  = FRAME_OK;
        int         decode_ret = 0;

        if (!is_lost[i]) {
            /* ===== 正常到达的包 ===== */
            stats.recv_pkts++;
            if (pkt_size <= 1) {
                memset(pcm_out, 0, pcm_buf_size * sizeof(int16_t));
                decode_ret = frame_samples;
                stats.dtx_frames++;
            } else {
                decode_ret = opus_decode(dec, enc_pkts[i], pkt_size,
                                          pcm_out, frame_samples, 0);
                if (decode_ret < 0)
                    fprintf(stderr, "解码错误 frame=%d: %s\n",
                            i, opus_strerror(decode_ret));
            }
            fstatus = FRAME_OK;

            /* 写输出、打印、记录 */
            stats.frame_status[i] = FRAME_OK;
            if (!cfg.no_audio_output && decode_ret > 0)
                wav_write_samples(&wf_out, pcm_out, decode_ret);
            if (cfg.verbose)
                printf("%-6d %-8d %-6s %-10s\n", i, pkt_size, "no", "OK");
            if (csv_fp)
                fprintf(csv_fp, "%d,%d,0,OK\n", i, pkt_size);
            i++;

        } else {
            /* ===== 丢包突发：找到突发边界 ===== */
            int burst_start = i;
            int burst_end   = i;
            while (burst_end + 1 < num_frames && is_lost[burst_end + 1])
                burst_end++;
            /* 找到突发后第一个到达的包 */
            int next_ok = burst_end + 1;
            /* 注意: next_ok 可能超出 num_frames（文件末尾的丢包突发）*/

            /* --- 预解析 DRED（如果可用）--- */
            int dred_parsed_samples = 0;
            if (cfg.dec_cfg.use_dred && dred_dec && dred &&
                next_ok < num_frames && enc_sizes[next_ok] > 1) {
                /* max_dred_samples = 从突发起点到达到包的距离（最大需求）*/
                int max_need = (next_ok - burst_start) * frame_samples;
                /* Opus DRED覆盖窗口最大 48000 样本 (1秒) */
                if (max_need > 48000) max_need = 48000;
                int dred_end_out = 0;
                int ret = opus_dred_parse(
                    dred_dec, dred,
                    enc_pkts[next_ok], enc_sizes[next_ok],
                    max_need, sr, &dred_end_out, 0);
                if (ret > 0) {
                    dred_parsed_samples = ret;
                }
            }

            /* --- 逐帧恢复突发中的每一帧 --- */
            for (int j = burst_start; j <= burst_end; j++) {
                int jsize = enc_sizes[j];
                stats.frame_size_bytes[j] = jsize;
                /* 已在外层计算过，避免重复累加 */
                stats.lost_pkts++;
                int recovered = 0;

                /* 方法1: LBRR — 仅适用于突发中最近的丢包帧
                 * （opus_decode decode_fec=1 从 next_ok 包恢复 burst_end）*/
                if (!recovered && j == burst_end &&
                    cfg.dec_cfg.use_lbrr && cfg.enc_cfg.use_fec &&
                    next_ok < num_frames && enc_sizes[next_ok] > 1 &&
                    opus_packet_has_lbrr(enc_pkts[next_ok], enc_sizes[next_ok])) {
                    decode_ret = opus_decode(dec,
                                             enc_pkts[next_ok], enc_sizes[next_ok],
                                             pcm_out, frame_samples, 1);
                    if (decode_ret > 0) {
                        fstatus = FRAME_LBRR;
                        recovered = 1;
                        stats.recovered_lbrr++;
                    }
                }

                /* 方法2: DRED
                 * dred_offset = (next_ok - j) * frame_samples
                 * 这遵循 opus_demo 中 (lost_count - fr) * output_samples 的逻辑 */
                if (!recovered && dred_parsed_samples > 0) {
                    opus_int32 dred_off = (opus_int32)((next_ok - j) * frame_samples);
                    decode_ret = opus_decoder_dred_decode(
                        dec, dred, dred_off, pcm_out, frame_samples);
                    if (decode_ret > 0) {
                        fstatus = FRAME_DRED;
                        recovered = 1;
                        stats.recovered_dred++;
                    }
                }

                /* 方法3: PLC */
                if (!recovered && cfg.dec_cfg.use_plc) {
                    decode_ret = opus_decode(dec, NULL, 0,
                                              pcm_out, frame_samples, 0);
                    if (decode_ret > 0) {
                        fstatus = FRAME_PLC;
                        stats.plc_frames++;
                        recovered = 1;
                    }
                }

                if (!recovered) {
                    memset(pcm_out, 0, pcm_buf_size * sizeof(int16_t));
                    decode_ret = frame_samples;
                    fstatus    = FRAME_LOST;
                }

                stats.frame_status[j] = (int)fstatus;
                if (!cfg.no_audio_output && decode_ret > 0)
                    wav_write_samples(&wf_out, pcm_out, decode_ret);

                if (cfg.verbose) {
                    const char *ss[] = {"OK","LBRR","DRED","PLC","LOST"};
                    printf("%-6d %-8d %-6s %-10s\n",
                           j, jsize, "YES", ss[(int)fstatus]);
                }
                if (csv_fp) {
                    const char *ss[] = {"OK","LBRR","DRED","PLC","LOST"};
                    fprintf(csv_fp, "%d,%d,1,%s\n", j, jsize, ss[(int)fstatus]);
                }
            }

            /* 对已统计的 total_encoded_bytes 修正
             * （burst 中每帧的 pkt_size 已在进入 while 循环时计入了第一帧,
             *   但 burst_start+1..burst_end 未计入，这里补上） */
            for (int j = burst_start + 1; j <= burst_end; j++)
                stats.total_encoded_bytes += enc_sizes[j];

            i = burst_end + 1;  /* 跳到突发结束后 */
        }
    }

    /* ---- 关闭输出 ---- */
    if (!cfg.no_audio_output) wav_close_write(&wf_out);
    if (csv_fp) fclose(csv_fp);

    /* ---- 打印统计 ---- */
    stats_print(&stats);

    /* 打印恢复效果 */
    if (stats.lost_pkts > 0) {
        int recovered_total = stats.recovered_lbrr + stats.recovered_dred;
        printf("丢包恢复率  : %.1f%% (%d/%d)\n",
               100.0 * recovered_total / stats.lost_pkts,
               recovered_total, stats.lost_pkts);
        printf("  - LBRR恢复: %d 帧\n", stats.recovered_lbrr);
        printf("  - DRED恢复: %d 帧\n", stats.recovered_dred);
        printf("  - PLC隐藏 : %d 帧\n", stats.plc_frames);
    }

    double duration_sec = (double)num_frames * frame_ms / 1000.0;
    if (duration_sec > 0 && stats.total_encoded_bytes > 0) {
        printf("平均码率    : %.1f kbps\n",
               8.0 * stats.total_encoded_bytes / duration_sec / 1000.0);
    }

    if (cfg.stats_csv[0])
        printf("统计CSV已保存: %s\n", cfg.stats_csv);

    /* ---- 清理 ---- */
    for (int i = 0; i < num_frames; i++)
        free(enc_pkts[i]);
    free(enc_pkts);
    free(enc_sizes);
    free(is_lost);
    free(pcm_in);
    free(pcm_out);
    free(enc_buf);
    free(stats.frame_status);
    free(stats.frame_size_bytes);

    if (dred)     opus_dred_free(dred);
    if (dred_dec) opus_dred_decoder_destroy(dred_dec);
    opus_encoder_destroy(enc);
    opus_decoder_destroy(dec);
    if (blob_data) free(blob_data);

    return 0;
}
