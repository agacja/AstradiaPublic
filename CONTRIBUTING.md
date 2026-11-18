# Contributing

Keep contributions scoped to the published components. Explain the problem,
the change, and how it was verified. Use synthetic data in examples and tests.

Never copy the private application or its history into this repository. Do not
include formulas, model parameters, strategy outputs, production account data,
environment files, logs, or credentials. See [PUBLIC-BOUNDARY.md](docs/PUBLIC-BOUNDARY.md).

The owner reviews publication scope and updates to `public-files.json`. A failed
manifest check means the proposed files need explicit review; do not remove the
check or blindly regenerate the manifest to make CI pass.

Run the relevant frontend, Go, or Rust checks and the publication check before
pushing. Changes to the application are not automatically synchronized to the
hosted Astradia product.
