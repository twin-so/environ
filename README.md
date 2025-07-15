# environ: manage secrets in git checkouts

## Design principles

- Secrets are stored in Google Cloud Storage and/or Amazon S3 as content-addressed ZIP files.
- A reference to the secrets corresponding to a given commit is stored in that commit.
- Configuration is done in [Starlark](https://starlark-lang.org/). See [how we use it as of 2025-07-06](example/environ.star) and [test cases](tests/environ.star).

## Execution

Finds `environ.star` in ancestors of the working directory and executes there.

### `environ pull [environ…]`

Reads the secrets reference(s), pulls the secrets from the remote(s), and installs them in the working directory.

Designed to run in [a `post-checkout` Git hook](example/post-checkout) or invoked manually.

### `environ push [environ…]`

Reads the secrets from the working directory, writes an archive to each environ's remote, and updates its reference file.

The reference files are ready to be committed.

### `environ diff [-from …] [-to …] [environ…]`

Reads the secrets from the working directory unless `-to` specifies otherwise, the secrets from the remote based on the current reference unless `-from` specifies otherwise, and outputs the difference.

# Similar projects

- [Keepass-2-file](https://github.com/Dracks/keepass-2-file): Build .env or any other plain text config file pulling the secrets from a keepass file
