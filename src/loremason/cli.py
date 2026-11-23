"""The `mason` command."""

import argparse
import sys

from loremason import __version__


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="mason",
        description="Store, check and query a context graph of typed, linked records.",
    )
    parser.add_argument("--version", action="version", version=f"mason {__version__}")
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    parser.parse_args(argv)
    parser.print_help()
    return 0


if __name__ == "__main__":
    sys.exit(main())
