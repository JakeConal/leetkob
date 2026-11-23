# Spec Delta

## Purpose

Make constraint predictions reproducible through one dataset-independent checker command, with unchanged official inputs and honest PA#3 evaluation evidence.

## ADDED Requirements

### Requirement: Dataset-independent checker image

The repository SHALL document one command taking a dataset directory and writing one valid JSONL prediction per solution/constraint pair, plus a Dockerfile and build/run lines. Predictions SHALL derive only from statements, constraints and source, never labels or scorer data. New problem IDs SHALL require no per-problem configuration.

#### Scenario: PA3-EVAL-01 Unfamiliar dataset

- **WHEN** the image receives an unseen problem following the official layout
- **THEN** the same command writes solution/constraint/verdict/original-line evidence without changing code or prompts for that problem

### Requirement: Model-off offline operation

MODEL_OFF=1 SHALL disable all model/network calls, decide ban-list pairs with hard rules and emit unsure for every semantic pair. The checker image SHALL work with network disabled, constrained memory/processes and no Docker socket. Its run SHALL complete within the marking command's ten-minute limit.

#### Scenario: PA3-EVAL-02 Marker's offline image run

- **WHEN** the marker runs the documented image with MODEL_OFF=1 and network none on the published dev inputs
- **THEN** it writes valid predictions without credentials or nested container access and unmodified eval.mjs exits 0 or 1, never invalid-input exit 2

### Requirement: Published data integrity and separate team cases

The dev dataset and scorer SHALL remain byte-for-byte the published version. Team cases SHALL occupy a separate folder in the same layout and include at least five labelled/tagged cases with at least one trap and one injection. Labels SHALL NOT be changed to fit predictions.

#### Scenario: PA3-EVAL-03 Sources and team cases are inspected

- **WHEN** the submitted tag is compared with the official hash manifest and separate team case inventory
- **THEN** official bytes match, team cases satisfy the count/tags and source provenance is recorded without overlaying official files

### Requirement: Committed first real-model evaluation

The submitted revision SHALL include real-model dev predictions, the unmodified scorer's report and provider call log. Report the three actual metrics without inventing a PA#3 overall pass threshold. README and AI-LOG SHALL name the model and show the arithmetic for one lab of 80 submissions.

#### Scenario: PA3-EVAL-04 Marker rescores committed predictions

- **WHEN** the marker scores the committed prediction file using their unchanged scorer
- **THEN** the metrics match the reported run and the record identifies source/model/prompt/data revisions and measured costs

#### Scenario: PA3-EVAL-05 First-run thresholds are missed

- **WHEN** valid first-run predictions miss recall or unsure thresholds
- **THEN** PA#3 reporting retains the actual metrics and scorer outcome without editing labels or claiming the future PA#4 gate has passed
