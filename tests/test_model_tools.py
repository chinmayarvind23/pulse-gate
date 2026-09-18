import pytest

from scripts.evidence import REPO, external_directory
from scripts.train_model import features


def test_results_cannot_enter_repository():
    """Evidence paths must stay outside source even when reached through '..'."""
    with pytest.raises(ValueError):
        external_directory(str(REPO / "docs" / ".." / "results"))


def test_external_output_allowed(tmp_path):
    """A normal artifact destination must be creatable without repo-specific layout."""
    assert external_directory(str(tmp_path / "evidence")) == tmp_path / "evidence"


def test_feature_contract_saturates_counts():
    """The export's input order and saturation match the documented serving contract."""
    event = {
        "amount_cents": 10**15,
        "hour_utc": 23,
        "card_present": False,
        "country": "GB",
        "velocity_5m": 10000,
        "velocity_1h": 10000,
        "prior_declines": 10000,
    }
    assert features(event) == [10, 1, 1, 1, 10, 10, 10]
