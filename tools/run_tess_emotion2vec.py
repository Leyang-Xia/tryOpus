#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import os
import re
import sys
from collections import Counter
from datetime import datetime
from pathlib import Path
from typing import Any

from funasr import AutoModel
from sklearn.metrics import accuracy_score, classification_report, confusion_matrix


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_TESS_DIR = ROOT / ".cache" / "datasets" / "tess" / "tess"
DEFAULT_RESULTS_DIR = ROOT / "results" / "emotion_eval"
DEFAULT_MODEL = "iic/emotion2vec_plus_large"
LOCAL_MODELSCOPE_MODEL = ROOT / ".cache" / "modelscope" / "models" / "iic" / "emotion2vec_plus_large"
LABEL_ORDER = ["neutral", "happy", "sad", "angry", "fear", "disgust", "ps"]
CANONICAL_LABELS = {
    "neutral": "neutral",
    "happy": "happy",
    "sad": "sad",
    "angry": "angry",
    "anger": "angry",
    "fear": "fear",
    "fearful": "fear",
    "disgust": "disgust",
    "disgusted": "disgust",
    "ps": "ps",
    "surprised": "ps",
    "surprise": "ps",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Evaluate emotion2vec on TESS.")
    parser.add_argument("--tess-dir", type=Path, default=DEFAULT_TESS_DIR)
    parser.add_argument("--results-dir", type=Path, default=DEFAULT_RESULTS_DIR)
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--batch-size", type=int, default=64)
    parser.add_argument("--limit", type=int, default=0)
    parser.add_argument("--run-name", default="")
    return parser.parse_args()


def collect_samples(tess_dir: Path) -> list[dict[str, str]]:
    wavs = sorted(tess_dir.glob("*.wav"))
    if not wavs:
        raise FileNotFoundError(f"No wav files found under {tess_dir}")

    samples: list[dict[str, str]] = []
    for wav_path in wavs:
        stem = wav_path.stem
        speaker, word, raw_label = stem.split("_", 2)
        samples.append(
            {
                "utt_id": stem,
                "path": str(wav_path),
                "speaker": speaker,
                "word": word,
                "label": raw_label,
            }
        )
    return samples


def normalize_label(label: str) -> str:
    cleaned = label.strip().lower()
    cleaned = re.sub(r"^[^a-z]+|[^a-z]+$", "", cleaned)
    if "/" in label:
        cleaned = label.split("/")[-1].strip().lower()
    return CANONICAL_LABELS.get(cleaned, cleaned)


def parse_prediction(item: Any) -> tuple[str, float, str]:
    if isinstance(item, list) and item:
        item = item[0]

    if isinstance(item, dict):
        for key in ("labels", "scores", "text"):
            if key not in item:
                continue

        labels = item.get("labels")
        scores = item.get("scores")
        if isinstance(labels, list) and labels:
            if isinstance(scores, list) and scores:
                max_index = max(range(len(scores)), key=lambda idx: float(scores[idx]))
                label = str(labels[max_index])
                score = float(scores[max_index])
            else:
                label = str(labels[0])
                score = 0.0
            return normalize_label(label), score, json.dumps(item, ensure_ascii=False)

        text = item.get("text")
        if isinstance(text, str) and text:
            score = 0.0
            if isinstance(scores, list) and scores:
                score = float(scores[0])
            return normalize_label(text), score, json.dumps(item, ensure_ascii=False)

    if isinstance(item, str):
        return normalize_label(item), 0.0, item

    raise ValueError(f"Unexpected prediction payload: {item!r}")


def write_wav_scp(samples: list[dict[str, str]], path: Path) -> None:
    with path.open("w", encoding="utf-8") as f:
        for sample in samples:
            f.write(f"{sample['utt_id']} {sample['path']}\n")


def run_inference(model: AutoModel, samples: list[dict[str, str]], batch_size: int, work_dir: Path) -> list[dict[str, Any]]:
    wav_scp = work_dir / "wav.scp"
    write_wav_scp(samples, wav_scp)
    raw = model.generate(
        str(wav_scp),
        granularity="utterance",
        extract_embedding=False,
        batch_size_s=batch_size,
    )

    if not isinstance(raw, list):
        raw = [raw]

    by_key: dict[str, dict[str, Any]] = {}
    for item in raw:
        if not isinstance(item, dict):
            raise ValueError(f"Unexpected inference item: {item!r}")
        key = str(item.get("key") or item.get("utt") or item.get("name") or "")
        pred, score, raw_payload = parse_prediction(item)
        by_key[key] = {
            "predicted_label": pred,
            "score": score,
            "raw_prediction": raw_payload,
        }

    rows: list[dict[str, Any]] = []
    missing: list[str] = []
    for sample in samples:
        result = by_key.get(sample["utt_id"])
        if result is None:
            missing.append(sample["utt_id"])
            continue
        rows.append({**sample, **result})

    if missing:
        raise RuntimeError(f"Missing predictions for {len(missing)} items, first few: {missing[:5]}")
    return rows


def make_run_dir(results_dir: Path, run_name: str) -> Path:
    if not run_name:
        run_name = f"emotion2vec_tess_{datetime.now().strftime('%Y%m%d_%H%M%S')}"
    run_dir = results_dir / run_name
    run_dir.mkdir(parents=True, exist_ok=True)
    return run_dir


def build_summary(rows: list[dict[str, Any]]) -> dict[str, Any]:
    y_true = [normalize_label(row["label"]) for row in rows]
    y_pred = [row["predicted_label"] for row in rows]
    accuracy = float(accuracy_score(y_true, y_pred))
    cm = confusion_matrix(y_true, y_pred, labels=LABEL_ORDER)
    report = classification_report(
        y_true,
        y_pred,
        labels=LABEL_ORDER,
        output_dict=True,
        zero_division=0,
    )
    return {
        "num_samples": len(rows),
        "accuracy": accuracy,
        "label_order": LABEL_ORDER,
        "ground_truth_distribution": dict(Counter(y_true)),
        "prediction_distribution": dict(Counter(y_pred)),
        "confusion_matrix": cm.tolist(),
        "classification_report": report,
    }


def write_results(run_dir: Path, rows: list[dict[str, Any]], summary: dict[str, Any], model_name: str) -> None:
    csv_path = run_dir / "predictions.csv"
    fields = [
        "utt_id",
        "path",
        "speaker",
        "word",
        "label",
        "predicted_label",
        "score",
        "raw_prediction",
    ]
    with csv_path.open("w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)

    summary_path = run_dir / "summary.json"
    with summary_path.open("w", encoding="utf-8") as f:
        json.dump({"model": model_name, **summary}, f, ensure_ascii=False, indent=2)

    report_path = run_dir / "report.md"
    with report_path.open("w", encoding="utf-8") as f:
        f.write("# TESS Emotion Analysis\n\n")
        f.write(f"- Model: `{model_name}`\n")
        f.write(f"- Samples: `{summary['num_samples']}`\n")
        f.write(f"- Accuracy: `{summary['accuracy'] * 100:.2f}%`\n\n")
        f.write("## Per-class Metrics\n\n")
        f.write("| Label | Precision | Recall | F1 | Support |\n")
        f.write("| --- | ---: | ---: | ---: | ---: |\n")
        for label in LABEL_ORDER:
            metrics = summary["classification_report"].get(label, {})
            f.write(
                f"| {label} | {metrics.get('precision', 0):.4f} | {metrics.get('recall', 0):.4f} | {metrics.get('f1-score', 0):.4f} | {int(metrics.get('support', 0))} |\n"
            )
        f.write("\n## Confusion Matrix\n\n")
        f.write("| true \\\\ pred | " + " | ".join(LABEL_ORDER) + " |\n")
        f.write("| --- | " + " | ".join(["---:"] * len(LABEL_ORDER)) + " |\n")
        for label, row in zip(LABEL_ORDER, summary["confusion_matrix"]):
            f.write("| " + label + " | " + " | ".join(str(v) for v in row) + " |\n")


def main() -> int:
    args = parse_args()
    os.environ.setdefault("MODELSCOPE_CACHE", str(ROOT / ".cache" / "modelscope"))
    os.environ.setdefault("HF_HOME", str(ROOT / ".cache" / "huggingface"))
    model_name = args.model
    if model_name == DEFAULT_MODEL and LOCAL_MODELSCOPE_MODEL.exists():
        model_name = str(LOCAL_MODELSCOPE_MODEL)

    samples = collect_samples(args.tess_dir)
    if args.limit > 0:
        samples = samples[: args.limit]

    run_dir = make_run_dir(args.results_dir, args.run_name)
    model = AutoModel(model=model_name, disable_update=True)
    rows = run_inference(model, samples, args.batch_size, run_dir)
    summary = build_summary(rows)
    write_results(run_dir, rows, summary, model_name)

    print(json.dumps({"run_dir": str(run_dir), "accuracy": summary["accuracy"], "num_samples": summary["num_samples"], "model": model_name}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
