# Model tools

The worker embeds a versioned model artifact under `apps/risk-worker/model/`. `scripts.train_model` builds the same transform and exports a classifier. `evals.risk_eval` checks the running worker against that export. Both commands require an output directory outside this repository.

The checked-in JSONL file is a fixed synthetic input dataset. The tools reject output paths inside the source tree.
