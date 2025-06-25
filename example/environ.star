TFVARS=[
    "tofu/{}/terraform.tfvars".format(env)
    for env in [
        "prod",
        "sandbox",
        "common"
    ]
]

SIMPLE=[".env"] + [
    "{}/.env".format(path)
    for path in [
        "qonto-frontend",
        "agent",
        "backend",
        "jaiminho/benchmarks/smart_login_benchmark",
        "jaiminho/jaiminho_cli",
        "jaiminho/sirius",
        "provider-matcher",
        "telescope",
        "multiverse",
        "multiverse/enginelogger",
        "frontend",
        "invoice-operator",
    ]
]

TEST=[
    "{}/.env.test".format(path)
    for path in [
        "agent",
        "jaiminho/sirius",
        "multiverse/enginelogger",
    ]
]

PER_ENV=[
    "{}/.env.{}".format(path, env)
    for path in [
        "jaiminho/jaiminho_cli",
        "jaiminho/sirius",
        "multiverse",
        "multiverse/enginelogger",
        "frontend",
        "invoice-operator",
        "clickhouse",
    ]
    for env in [
        "prod",
        "sandbox",
        "minikube",
    ]
]

environ(
    name   = "monorepo",
    remote = cache(
        of = gcs(bucket = "twin-secrets", prefix = "environ-monorepo"),
        by = local(path = "~/.cache/environ-monorepo"),
    ),
    ref    = "environ.hash",
    files  = TFVARS + SIMPLE + TEST + PER_ENV,
)
