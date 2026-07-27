# CLA signatures

Storage branch for the CLA Assistant action configured in .github/workflows.
The action writes and updates signatures/cla.json on this branch as
contributors sign; it is not part of the source tree and never merges to main.

Keep this branch unprotected, otherwise the action cannot commit signatures.
