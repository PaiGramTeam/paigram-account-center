from __future__ import annotations

import shutil
from importlib.resources import files
from pathlib import Path

from grpc_tools import protoc

SDK_ROOT = Path(__file__).resolve().parents[1]
REPOSITORY_ROOT = SDK_ROOT.parents[1]
PROTO_ROOT = REPOSITORY_ROOT / "contracts" / "proto"
OUTPUT_ROOT = SDK_ROOT / "src" / "paigram_account_sdk" / "_generated"
PROTO_FILES = (
    "account/v1/bot_access.proto",
    "platform/v1/platform.proto",
    "mihomo/v1/credential.proto",
)


def main() -> int:
    if OUTPUT_ROOT.exists():
        shutil.rmtree(OUTPUT_ROOT)
    OUTPUT_ROOT.mkdir(parents=True)

    result = protoc.main(
        [
            "grpc_tools.protoc",
            f"--proto_path={PROTO_ROOT}",
            f"--proto_path={files('grpc_tools').joinpath('_proto')}",
            f"--python_out={OUTPUT_ROOT}",
            f"--pyi_out={OUTPUT_ROOT}",
            f"--grpc_python_out={OUTPUT_ROOT}",
            *PROTO_FILES,
        ]
    )
    if result != 0:
        return result

    for package in ("", "account", "account/v1", "platform", "platform/v1", "mihomo", "mihomo/v1"):
        (OUTPUT_ROOT / package / "__init__.py").touch()

    for generated_stub in OUTPUT_ROOT.rglob("*_pb2_grpc.py"):
        source = generated_stub.read_text(encoding="utf-8")
        for package in ("account", "platform", "mihomo"):
            source = source.replace(
                f"from {package}.v1 import ",
                f"from paigram_account_sdk._generated.{package}.v1 import ",
            )
        generated_stub.write_text(source, encoding="utf-8", newline="\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
