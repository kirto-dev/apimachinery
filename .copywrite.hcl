schema_version = 1

project {
  # Apache-2.0 matches the LICENSE at the repo root, which copywrite can
  # generate. The header is what keeps the claim attached to a file once
  # it leaves the repo.
  license          = "Apache-2.0"
  copyright_year   = 2026
  copyright_holder = "Kirto LLC"

  header_ignore = [
    # Fixtures are compared byte-for-byte.
    "**/testdata/**",

    # Prose and data: nothing here is a source file whose provenance the
    # header is protecting.
    "**/*.md",
    "**/*.json",
  ]
}
