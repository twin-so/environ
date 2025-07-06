# environ: manage secrets in git checkouts

## Design principles

- Secrets are stored in Google Cloud Storage and/or Amazon S3 as content-addressed ZIP files.
- A reference to the secrets corresponding to a given commit is stored in that commit.
- Configuration is done in [Starlark](https://starlark-lang.org/). See [how we use it as of 2025-07-06](example/environ.star) and [test cases](tests/environ.star).

## Execution

Finds `environ.star` in ancestors of the working directory and executes there.

### `environ pull`

Reads the secrets reference, pulls the secrets from the remote, and installs them in the working directory.

Designed to run in [a `post-checkout` Git hook](example/post-checkout) or invoked manually.

### `environ push`

Reads the secrets from the working directory, writes an archive to the remote, and updates the reference.

The reference file is ready to be committed.

### `environ diff`

Reads the secrets from the working directory, the secrets from the remote based on the current reference, and outputs the difference.
