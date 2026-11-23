import subprocess
import sys

import pytest

from loremason import __version__
from loremason.cli import main


def test_version_flag(capsys):
    with pytest.raises(SystemExit) as exc:
        main(["--version"])
    assert exc.value.code == 0
    assert capsys.readouterr().out.strip() == f"mason {__version__}"


def test_no_arguments_prints_help(capsys):
    assert main([]) == 0
    assert "usage: mason" in capsys.readouterr().out


def test_runs_as_module():
    out = subprocess.run(
        [sys.executable, "-m", "loremason.cli", "--version"],
        capture_output=True, text=True, check=True,
    )
    assert out.stdout.strip() == f"mason {__version__}"
