from __future__ import annotations

import logging
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
    "platform/v2/types.proto",
    "platform/v2/control.proto",
    "mihomo/v2/runtime.proto",
)
logger = logging.getLogger(__name__)


def main() -> int:
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    if OUTPUT_ROOT.exists():
        logger.info("Removing generated Python contracts from %s", OUTPUT_ROOT)
        shutil.rmtree(OUTPUT_ROOT)
    OUTPUT_ROOT.mkdir(parents=True)

    logger.info("Generating Python contracts from %d proto files", len(PROTO_FILES))
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
        logger.error("Python contract generation failed with exit code %d", result)
        return result

    package_directories = {Path()}
    for proto_file in PROTO_FILES:
        parent = Path(proto_file).parent
        package_directories.update(parent.parents)
        package_directories.add(parent)
    for package in package_directories:
        (OUTPUT_ROOT / package / "__init__.py").touch()

    top_level_packages = {Path(proto_file).parts[0] for proto_file in PROTO_FILES}
    generated_modules = (
        *OUTPUT_ROOT.rglob("*_pb2*.py"),
        *OUTPUT_ROOT.rglob("*_pb2*.pyi"),
    )
    for generated_module in generated_modules:
        source = generated_module.read_text(encoding="utf-8")
        for package in top_level_packages:
            for version in ("v1", "v2"):
                source = source.replace(
                    f"from {package}.{version} import ",
                    f"from paigram_account_sdk._generated.{package}.{version} import ",
                )
        generated_module.write_text(source, encoding="utf-8", newline="\n")
    logger.info("Python contract generation completed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
