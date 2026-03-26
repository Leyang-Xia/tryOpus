#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import os
import subprocess
import sys
from datetime import datetime
from pathlib import Path

import librosa
import soundfile as sf
from sklearn.metrics import accuracy_score, classification_report

from run_tess_emotion2vec import (
    DEFAULT_MODEL,
    LOCAL_MODELSCOPE_MODEL,
    AutoModel,
    build_summary,
    collect_samples,
    make_run_dir,
    run_inference,
)


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_TESS_DIR = ROOT / ".cache" / "datasets" / "tess" / "tess"
DEFAULT_RESULTS_DIR = ROOT / "results" / "emotion_eval"
SIM_BIN = ROOT / "build" / "opus_sim"
EMO_LABELS = ["neutral", "happy", "sad", "angry", "fear", "disgust"]

SCENARIOS = [
    ("clean", []),
    ("uniform_10", ["--loss", "0.10", "--no-lbrr", "--no-dred"]),
    ("uniform_20", ["--loss", "0.20", "--no-lbrr", "--no-dred"]),
    ("ge_light", ["-ge", "-ge-p2b", "0.03", "-ge-b2g", "0.50", "-ge-bloss", "0.60", "--no-lbrr", "--no-dred"]),
    ("ge_moderate", ["-ge", "-ge-p2b", "0.05", "-ge-b2g", "0.30", "-ge-bloss", "0.80", "--no-lbrr", "--no-dred"]),
    ("ge_heavy", ["-ge", "-ge-p2b", "0.10", "-ge-b2g", "0.15", "-ge-bloss", "0.90", "--no-lbrr", "--no-dred"]),
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run offline Opus + emotion2vec evaluation on TESS.")
    parser.add_argument("--tess-dir", type=Path, default=DEFAULT_TESS_DIR)
    parser.add_argument("--results-dir", type=Path, default=DEFAULT_RESULTS_DIR)
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--batch-size", type=int, default=64)
    parser.add_argument("--limit", type=int, default=0)
    parser.add_argument("--run-name", default="")
    parser.add_argument("--bitrate", type=int, default=32000)
    parser.add_argument("--framesize", type=int, default=20)
    parser.add_argument("--complexity", type=int, default=9)
    parser.add_argument("--signal", default="voice")
    return parser.parse_args()


def resolve_model_name(model_name: str) -> str:
    if model_name == DEFAULT_MODEL and LOCAL_MODELSCOPE_MODEL.exists():
        return str(LOCAL_MODELSCOPE_MODEL)
    return model_name


def filter_samples(samples: list[dict[str, str]]) -> list[dict[str, str]]:
    return [sample for sample in samples if sample["label"] != "ps"]


def prepare_inputs(samples: list[dict[str, str]], prepared_dir: Path) -> list[dict[str, str]]:
    prepared_dir.mkdir(parents=True, exist_ok=True)
    prepared: list[dict[str, str]] = []
    for sample in samples:
        output_path = prepared_dir / f"{sample['utt_id']}.wav"
        if not output_path.exists():
            audio, _ = librosa.load(sample["path"], sr=24000, mono=True)
            sf.write(output_path, audio, 24000, subtype="PCM_16")
        prepared.append({**sample, "path": str(output_path)})
    return prepared


def simulate_one(
    sim_bin: Path,
    sample: dict[str, str],
    output_wav: Path,
    scenario_args: list[str],
    bitrate: int,
    framesize: int,
    complexity: int,
    signal: str,
) -> None:
    cmd = [
        str(sim_bin),
        "--bitrate",
        str(bitrate),
        "--framesize",
        str(framesize),
        "--complexity",
        str(complexity),
        "--signal",
        signal,
        "--csv",
        str(output_wav.with_suffix(".csv")),
    ]
    cmd.extend(scenario_args)
    cmd.extend([sample["path"], str(output_wav)])

    env = os.environ.copy()
    opus_lib = ROOT / "opus-install" / "lib"
    env["LD_LIBRARY_PATH"] = f"{opus_lib}:{env.get('LD_LIBRARY_PATH', '')}"
    env["DYLD_LIBRARY_PATH"] = f"{opus_lib}:{env.get('DYLD_LIBRARY_PATH', '')}"
    subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env=env)


def write_eval_csv(rows: list[dict[str, object]], path: Path) -> None:
    with path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(
            f,
            fieldnames=[
                "scenario",
                "utt_id",
                "speaker",
                "word",
                "label",
                "predicted_label",
                "score",
                "audio_path",
            ],
        )
        writer.writeheader()
        writer.writerows(rows)


def aggregate_rows(scenario: str, samples: list[dict[str, str]], prediction_rows: list[dict[str, object]]) -> list[dict[str, object]]:
    aggregated: list[dict[str, object]] = []
    by_utt = {row["utt_id"]: row for row in prediction_rows}
    for sample in samples:
        row = by_utt[sample["utt_id"]]
        aggregated.append(
            {
                "scenario": scenario,
                "utt_id": sample["utt_id"],
                "speaker": sample["speaker"],
                "word": sample["word"],
                "label": sample["label"],
                "predicted_label": row["predicted_label"],
                "score": row["score"],
                "audio_path": row["path"],
            }
        )
    return aggregated


def write_report(run_dir: Path, scenario_summaries: list[dict[str, object]], model_name: str, sample_count: int) -> None:
    report_path = run_dir / "report.md"
    with report_path.open("w", encoding="utf-8") as f:
        f.write("# TESS Offline Opus Emotion Evaluation\n\n")
        f.write(f"- Model: `{model_name}`\n")
        f.write(f"- Dataset: `TESS` (`ps` excluded)\n")
        f.write(f"- Samples per scenario: `{sample_count}`\n\n")
        f.write("## Scenario Summary\n\n")
        f.write("| Scenario | Accuracy | Accuracy Drop vs Clean | Macro F1 |\n")
        f.write("| --- | ---: | ---: | ---: |\n")
        for item in scenario_summaries:
            f.write(
                f"| {item['scenario']} | {item['accuracy'] * 100:.2f}% | {item['accuracy_drop_pp']:.2f} pp | {item['macro_f1']:.4f} |\n"
            )


def main() -> int:
    args = parse_args()
    if not SIM_BIN.exists():
        raise FileNotFoundError(f"Missing opus_sim: {SIM_BIN}")

    os.environ.setdefault("MODELSCOPE_CACHE", str(ROOT / ".cache" / "modelscope"))
    os.environ.setdefault("HF_HOME", str(ROOT / ".cache" / "huggingface"))

    model_name = resolve_model_name(args.model)
    samples = filter_samples(collect_samples(args.tess_dir))
    if args.limit > 0:
        samples = samples[: args.limit]

    if not args.run_name:
        args.run_name = f"tess_offline_emotion_{datetime.now().strftime('%Y%m%d_%H%M%S')}"
    run_dir = make_run_dir(args.results_dir, args.run_name)
    generated_root = run_dir / "generated"
    transcripts_root = run_dir / "transcripts"
    prepared_inputs_root = run_dir / "prepared_inputs"
    generated_root.mkdir(parents=True, exist_ok=True)
    transcripts_root.mkdir(parents=True, exist_ok=True)
    prepared_inputs_root.mkdir(parents=True, exist_ok=True)
    model = AutoModel(model=model_name, disable_update=True)
    prepared_samples = prepare_inputs(samples, prepared_inputs_root)

    model_rows_by_scenario: dict[str, list[dict[str, object]]] = {}
    all_eval_rows: list[dict[str, object]] = []

    for scenario_name, scenario_args in SCENARIOS:
        scenario_audio_dir = generated_root / scenario_name
        scenario_transcript_dir = transcripts_root / scenario_name
        scenario_audio_dir.mkdir(parents=True, exist_ok=True)
        scenario_transcript_dir.mkdir(parents=True, exist_ok=True)
        scenario_samples: list[dict[str, str]] = []

        if scenario_name == "clean":
            for sample in prepared_samples:
                scenario_samples.append({**sample, "path": sample["path"]})
        else:
            for idx, sample in enumerate(prepared_samples, 1):
                output_wav = scenario_audio_dir / f"{sample['utt_id']}.wav"
                simulate_one(
                    SIM_BIN,
                    sample,
                    output_wav,
                    scenario_args,
                    args.bitrate,
                    args.framesize,
                    args.complexity,
                    args.signal,
                )
                scenario_samples.append({**sample, "path": str(output_wav)})
                if idx % 200 == 0:
                    print(f"[sim] {scenario_name}: {idx}/{len(prepared_samples)}")

        prediction_rows = run_inference(
            model=model,
            samples=scenario_samples,
            batch_size=args.batch_size,
            work_dir=scenario_transcript_dir,
        )
        # run_inference expects a model object; create lazily inside a wrapper call
        model_rows_by_scenario[scenario_name] = prediction_rows
        all_eval_rows.extend(aggregate_rows(scenario_name, scenario_samples, prediction_rows))

    clean_rows = model_rows_by_scenario["clean"]
    clean_true = [row["label"] for row in clean_rows]
    clean_pred = [str(row["predicted_label"]) for row in clean_rows]
    clean_accuracy = accuracy_score(clean_true, clean_pred)

    scenario_summaries: list[dict[str, object]] = []
    for scenario_name, rows in model_rows_by_scenario.items():
        y_true = [row["label"] for row in rows]
        y_pred = [str(row["predicted_label"]) for row in rows]
        cls_report = classification_report(
            y_true,
            y_pred,
            labels=EMO_LABELS,
            output_dict=True,
            zero_division=0,
        )
        macro_f1 = cls_report["macro avg"]["f1-score"]
        accuracy = accuracy_score(y_true, y_pred)
        scenario_summaries.append(
            {
                "scenario": scenario_name,
                "accuracy": accuracy,
                "accuracy_drop_pp": (clean_accuracy - accuracy) * 100.0,
                "macro_f1": macro_f1,
            }
        )

    with (run_dir / "scenario_summary.json").open("w", encoding="utf-8") as f:
        json.dump(
            {
                "model": model_name,
                "samples_per_scenario": len(prepared_samples),
                "scenario_summaries": scenario_summaries,
            },
            f,
            ensure_ascii=False,
            indent=2,
        )

    write_eval_csv(all_eval_rows, run_dir / "emotion_predictions.csv")
    write_report(run_dir, scenario_summaries, model_name, len(prepared_samples))
    print(json.dumps({"run_dir": str(run_dir), "samples_per_scenario": len(prepared_samples), "scenarios": [name for name, _ in SCENARIOS]}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
