/**
 * netsim.h - 网络信道仿真接口
 *
 * 实现：
 *   - 均匀分布丢包 (Uniform)
 *   - Gilbert-Elliott 突发丢包模型 (更接近真实网络)
 *   - 时延 + 高斯抖动
 */
#pragma once

#include "common.h"

/* Gilbert-Elliott 状态 */
typedef enum { GE_STATE_GOOD = 0, GE_STATE_BAD = 1 } GEState;

/* 网络仿真器内部状态 */
typedef struct {
    NetSimConfig cfg;

    /* Gilbert-Elliott 当前状态 */
    GEState ge_state;

    /* 随机数状态 (使用 LCG，避免依赖外部库) */
    uint64_t rand_state;

    /* 统计 */
    int total_pkts;
    int lost_pkts;
} NetSim;

/* ========== 实现 ========== */

static inline void netsim_seed(NetSim *ns, uint64_t seed) {
    ns->rand_state = seed ^ 0xDEADBEEFCAFEULL;
}

/* 返回 [0,1) 的均匀随机数 */
static inline double netsim_rand(NetSim *ns) {
    /* xorshift64 */
    uint64_t x = ns->rand_state;
    x ^= x << 13;
    x ^= x >> 7;
    x ^= x << 17;
    ns->rand_state = x;
    return (double)(x >> 11) / (double)(1ULL << 53);
}

/* 高斯随机数（Box-Muller变换）*/
static inline double netsim_randn(NetSim *ns, double mean, double std) {
    if (std <= 0.0) return mean;
    double u1 = netsim_rand(ns);
    double u2 = netsim_rand(ns);
    if (u1 < 1e-10) u1 = 1e-10;
    double z = __builtin_sqrt(-2.0 * __builtin_log(u1))
               * __builtin_cos(2.0 * 3.14159265358979 * u2);
    return mean + std * z;
}

/* 初始化仿真器 */
static inline void netsim_init(NetSim *ns, const NetSimConfig *cfg) {
    memset(ns, 0, sizeof(*ns));
    ns->cfg       = *cfg;
    ns->ge_state  = GE_STATE_GOOD;
    netsim_seed(ns, 12345678ULL);
}

/**
 * 判断当前包是否丢失
 * 返回 1 = 丢失, 0 = 通过
 */
static inline int netsim_is_lost(NetSim *ns) {
    ns->total_pkts++;
    double r = netsim_rand(ns);
    int lost = 0;

    switch (ns->cfg.loss_model) {
    case LOSS_UNIFORM:
        lost = (r < ns->cfg.loss_rate) ? 1 : 0;
        break;

    case LOSS_GILBERT: {
        /* 状态转移 */
        double tr = netsim_rand(ns);
        if (ns->ge_state == GE_STATE_GOOD) {
            if (tr < ns->cfg.p_good_to_bad)
                ns->ge_state = GE_STATE_BAD;
        } else {
            if (tr < ns->cfg.p_bad_to_good)
                ns->ge_state = GE_STATE_GOOD;
        }
        /* 根据状态决定丢失 */
        float loss_prob = (ns->ge_state == GE_STATE_GOOD)
                          ? ns->cfg.loss_in_good
                          : ns->cfg.loss_in_bad;
        lost = (r < loss_prob) ? 1 : 0;
        break;
    }

    default:
        lost = 0;
    }

    if (lost) ns->lost_pkts++;
    return lost;
}

/**
 * 计算当前包的仿真时延（毫秒）
 * 返回 base_delay + gaussian_jitter（截断到>=0）
 */
static inline double netsim_delay_ms(NetSim *ns) {
    double d = netsim_randn(ns,
                            ns->cfg.base_delay_ms,
                            ns->cfg.jitter_std_ms);
    return d < 0.0 ? 0.0 : d;
}

/**
 * 打印 Gilbert-Elliott 模型的统计特性
 * 平均突发长度 = 1/p_bad_to_good
 * 稳态丢包率 = p_good_to_bad/(p_good_to_bad+p_bad_to_good)*loss_bad
 *              + p_bad_to_good/(p_good_to_bad+p_bad_to_good)*loss_good
 */
static inline void netsim_print_params(const NetSimConfig *cfg) {
    printf("--- 网络仿真参数 ---\n");
    if (cfg->loss_model == LOSS_UNIFORM) {
        printf("模型         : 均匀丢包\n");
        printf("丢包率       : %.1f%%\n", cfg->loss_rate * 100.0f);
    } else if (cfg->loss_model == LOSS_GILBERT) {
        float pi_bad   = cfg->p_good_to_bad /
                         (cfg->p_good_to_bad + cfg->p_bad_to_good);
        float mean_loss = pi_bad * cfg->loss_in_bad +
                          (1 - pi_bad) * cfg->loss_in_good;
        float burst_len = cfg->p_bad_to_good > 0 ?
                          1.0f / cfg->p_bad_to_good : 999.0f;
        printf("模型         : Gilbert-Elliott\n");
        printf("期望丢包率   : %.1f%%\n", mean_loss * 100.0f);
        printf("平均突发长度 : %.1f 包\n", burst_len);
    }
    printf("基础时延     : %.1f ms\n", cfg->base_delay_ms);
    printf("抖动标准差   : %.1f ms\n", cfg->jitter_std_ms);
    printf("-------------------\n");
}

/* 获取实际丢包率 */
static inline float netsim_actual_loss_rate(const NetSim *ns) {
    if (ns->total_pkts == 0) return 0.0f;
    return (float)ns->lost_pkts / ns->total_pkts;
}
