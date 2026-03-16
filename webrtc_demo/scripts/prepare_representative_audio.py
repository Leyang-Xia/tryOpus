#!/usr/bin/env python3
"""Thin wrapper: delegates to tools/prepare_representative_audio.py."""
import importlib.util
import pathlib
import sys

_tools = pathlib.Path(__file__).resolve().parent.parent.parent / "tools" / "prepare_representative_audio.py"
spec = importlib.util.spec_from_file_location("prepare_representative_audio", str(_tools))
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)

if __name__ == "__main__":
    mod.main()
