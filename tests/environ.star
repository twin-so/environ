def case(name, remote):
    environ(
        name   = name,
        remote = cache(of = remote, by = local(path = name+".cache")),
        ref    = name + ".hash",
        files  = ["empty"],
    )

for i in range(2):
    s=str(i+1)
    case("gcs-"+s, gcs(bucket="twin-environ-tests", prefix=s))
    case("s3-"+s, s3(bucket="twin-environ-tests", prefix=s, region="eu-west-1", profile="twin-sandbox-admin"))
    case("fs-"+s, fs(path="tests/store", prefix=s))
