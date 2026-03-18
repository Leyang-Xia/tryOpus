/**
 * wav_io.h - 简单的WAV文件读写接口
 * 支持 16-bit PCM mono/stereo
 */
#pragma once

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#pragma pack(push, 1)
typedef struct {
    /* RIFF chunk */
    char     riff_id[4];       /* "RIFF" */
    uint32_t riff_size;        /* 文件大小 - 8 */
    char     wave_id[4];       /* "WAVE" */
    /* fmt sub-chunk */
    char     fmt_id[4];        /* "fmt " */
    uint32_t fmt_size;         /* 16 for PCM */
    uint16_t audio_format;     /* 1 = PCM */
    uint16_t num_channels;
    uint32_t sample_rate;
    uint32_t byte_rate;        /* sample_rate * num_channels * bits/8 */
    uint16_t block_align;      /* num_channels * bits/8 */
    uint16_t bits_per_sample;  /* 16 */
    /* data sub-chunk */
    char     data_id[4];       /* "data" */
    uint32_t data_size;        /* 实际PCM数据字节数 */
} WAVHeader;
#pragma pack(pop)

typedef struct {
    FILE     *fp;
    WAVHeader hdr;
    int       num_samples;     /* 每声道采样数 */
    long      data_offset;     /* PCM数据在文件中的偏移 */
} WAVFile;

/* 打开WAV文件（只读）- 返回0成功，-1失败 */
static inline int wav_open_read(WAVFile *wf, const char *path) {
    wf->fp = fopen(path, "rb");
    if (!wf->fp) { perror(path); return -1; }

    /* 读 RIFF/WAVE 头（12字节）*/
    char riff[4], wave[4];
    uint32_t riff_size;
    if (fread(riff, 4, 1, wf->fp) != 1 ||
        fread(&riff_size, 4, 1, wf->fp) != 1 ||
        fread(wave, 4, 1, wf->fp) != 1) {
        fprintf(stderr, "WAV: 读取文件头失败: %s\n", path);
        fclose(wf->fp); return -1;
    }
    if (memcmp(riff, "RIFF", 4) || memcmp(wave, "WAVE", 4)) {
        fprintf(stderr, "WAV: 非RIFF/WAVE格式: %s\n", path);
        fclose(wf->fp); return -1;
    }
    memcpy(wf->hdr.riff_id, riff, 4);
    wf->hdr.riff_size = riff_size;
    memcpy(wf->hdr.wave_id, wave, 4);

    /* 按块逐一解析（支持任意块顺序和额外块）*/
    int found_fmt = 0, found_data = 0;
    char chunk_id[4];
    uint32_t chunk_size;

    while (!found_data) {
        if (fread(chunk_id, 4, 1, wf->fp) != 1) break;
        if (fread(&chunk_size, 4, 1, wf->fp) != 1) break;

        if (memcmp(chunk_id, "fmt ", 4) == 0) {
            uint8_t fmt_buf[40];
            uint32_t to_read = chunk_size < 40 ? chunk_size : 40;
            if (fread(fmt_buf, to_read, 1, wf->fp) != 1) break;
            if (chunk_size > to_read)
                fseek(wf->fp, (long)(chunk_size - to_read), SEEK_CUR);

            memcpy(wf->hdr.fmt_id, "fmt ", 4);
            wf->hdr.fmt_size        = chunk_size;
            wf->hdr.audio_format    = *(uint16_t *)(fmt_buf + 0);
            wf->hdr.num_channels    = *(uint16_t *)(fmt_buf + 2);
            wf->hdr.sample_rate     = *(uint32_t *)(fmt_buf + 4);
            wf->hdr.byte_rate       = *(uint32_t *)(fmt_buf + 8);
            wf->hdr.block_align     = *(uint16_t *)(fmt_buf + 12);
            wf->hdr.bits_per_sample = *(uint16_t *)(fmt_buf + 14);

            if (wf->hdr.audio_format != 1) {
                fprintf(stderr, "WAV: 仅支持PCM格式(format=%d): %s\n",
                        wf->hdr.audio_format, path);
                fclose(wf->fp); return -1;
            }
            found_fmt = 1;

        } else if (memcmp(chunk_id, "data", 4) == 0) {
            memcpy(wf->hdr.data_id, "data", 4);
            wf->hdr.data_size = chunk_size;
            wf->data_offset   = ftell(wf->fp);
            wf->num_samples   = (int)(chunk_size /
                (wf->hdr.num_channels * wf->hdr.bits_per_sample / 8));
            found_data = 1;
        } else {
            /* 跳过未知块（如 LIST, INFO 等）*/
            fseek(wf->fp, (long)chunk_size, SEEK_CUR);
        }
    }

    if (!found_fmt || !found_data) {
        fprintf(stderr, "WAV: 解析失败(fmt=%d,data=%d): %s\n",
                found_fmt, found_data, path);
        fclose(wf->fp); return -1;
    }
    return 0;
}

/* 创建WAV文件（只写）*/
static inline int wav_open_write(WAVFile *wf, const char *path,
                                  int sample_rate, int channels) {
    wf->fp = fopen(path, "wb");
    if (!wf->fp) { perror(path); return -1; }

    memset(&wf->hdr, 0, sizeof(WAVHeader));
    memcpy(wf->hdr.riff_id,  "RIFF", 4);
    memcpy(wf->hdr.wave_id,  "WAVE", 4);
    memcpy(wf->hdr.fmt_id,   "fmt ", 4);
    memcpy(wf->hdr.data_id,  "data", 4);
    wf->hdr.fmt_size        = 16;
    wf->hdr.audio_format    = 1;
    wf->hdr.num_channels    = (uint16_t)channels;
    wf->hdr.sample_rate     = (uint32_t)sample_rate;
    wf->hdr.bits_per_sample = 16;
    wf->hdr.block_align     = (uint16_t)(channels * 2);
    wf->hdr.byte_rate       = (uint32_t)(sample_rate * channels * 2);
    wf->hdr.data_size       = 0;
    wf->hdr.riff_size       = 36;  /* 将在关闭时更新 */
    wf->num_samples         = 0;

    if (fwrite(&wf->hdr, sizeof(WAVHeader), 1, wf->fp) != 1) {
        fclose(wf->fp); return -1;
    }
    wf->data_offset = ftell(wf->fp);
    return 0;
}

/* 读取PCM样本（int16），返回读取的帧数（每帧=所有声道）*/
static inline int wav_read_samples(WAVFile *wf, int16_t *buf, int num_frames) {
    int samples_to_read = num_frames * wf->hdr.num_channels;
    return (int)fread(buf, sizeof(int16_t), samples_to_read, wf->fp)
           / wf->hdr.num_channels;
}

/* 写入PCM样本（int16）*/
static inline int wav_write_samples(WAVFile *wf, const int16_t *buf, int num_frames) {
    int samples = num_frames * wf->hdr.num_channels;
    int written = (int)fwrite(buf, sizeof(int16_t), samples, wf->fp);
    wf->num_samples += written / wf->hdr.num_channels;
    return written / wf->hdr.num_channels;
}

/* 关闭并补全文件头 */
static inline void wav_close_write(WAVFile *wf) {
    if (!wf->fp) return;
    uint32_t data_bytes = (uint32_t)(wf->num_samples *
                          wf->hdr.num_channels * 2);
    wf->hdr.data_size = data_bytes;
    wf->hdr.riff_size = 36 + data_bytes;
    fseek(wf->fp, 0, SEEK_SET);
    fwrite(&wf->hdr, sizeof(WAVHeader), 1, wf->fp);
    fclose(wf->fp);
    wf->fp = NULL;
}

static inline void wav_close_read(WAVFile *wf) {
    if (wf->fp) { fclose(wf->fp); wf->fp = NULL; }
}
