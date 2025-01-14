# Hacking on Tilt

So you want to make a change to `tilt`!

## Contributing

We welcome contributions, either as bug reports, feature requests, or pull requests.

We want everyone to feel at home in this repo and its environs; please see our [**Code of Conduct**](https://docs.tilt.dev/code_of_conduct.html) for some rules that govern everyone's participation.

Most of this page describes how to get set up making & testing changes.

Small PRs are better than large ones. If you have an idea for a major feature, please file
an issue first. The [Roadmap](ROADMAP.md) has details on some of the upcoming
features that we have in mind and might already be in-progress.

## Build Prereqs

If you just want to build Tilt:

- **[make](https://www.gnu.org/software/make/)**
- **[go 1.12](https://golang.org/dl/)**
- **[errcheck](https://github.com/kisielk/errcheck)**: `go get -u github.com/kisielk/errcheck` (to run lint)
- [yarn](https://yarnpkg.com/lang/en/docs/install/) (for JS resources)

## Test Prereqs

If you want to run the tests:

- **[docker](https://docs.docker.com/install/)** - Many of the `tilt` build steps do work inside of containers
  so that you don't need to install extra toolchains locally (e.g., the protobuf compiler).
- **[kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)**
- **[kustomize 2.0 or higher](https://github.com/kubernetes-sigs/kustomize)**: `go get -u sigs.k8s.io/kustomize`
- **[helm](https://docs.helm.sh/using_helm/#installing-helm)**
- **[docker compose](https://docs.docker.com/compose/install/)**: NOTE: this doesn't need to be installed separately from Docker on macOS
- **[jq](https://stedolan.github.io/jq/download/)**

## Optional Prereqs

Other development commands:

- **[wire](https://github.com/google/wire)**: `go get -u github.com/google/wire/cmd/wire` (to update generated dependency injection code)
- **[goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)**: `go get -u golang.org/x/tools/cmd/goimports` (to sort imports, IDE-specific installation instructions in the link). You should configure goimports to run with `-local github.com/windmill/tilt`
- Our Python scripts are in Python 3.6.0. To run them:
  - **[pyenv](https://github.com/pyenv/pyenv#installation)**
  - **python**: `pyenv install`
  - if you're using GKE and get the error: "pyenv: python2: command not found", run:
    - `git clone git://github.com/concordusapps/pyenv-implict.git ~/.pyenv/plugins/pyenv-implict`

## Developing

To check out Tilt for the first time, run:

```
go get -u github.com/windmilleng/tilt/cmd/tilt
```

The Go toolchain will checkout the Tilt repo somewhere on your GOPATH,
usually under `~/go/src/github.com/windmilleng/tilt`. (See notes below if you're using Go modules).

To run the fast test suite, run:

```
make shorttest
```

To run the slow test suite that interacts with Docker and builds real images, run:

```
make test
```

If you want to run an integration test suite that deploys servers to Kubernetes and
verifies them, run:

```
make integration
```

To install `tilt` on PATH, run

```
make install
```

To start using Tilt, just run `tilt up` in any project with a `Tiltfile` -- i.e., NOT the root of the Tilt source code.
There are plenty of toy projects to play with in the [integration](https://github.com/windmilleng/tilt/tree/master/integration) directory
(see e.g. `./integration/oneup`), or check out one of these sample repos to get started:
- [ABC123](https://github.com/windmilleng/abc123): Go/Python/JavaScript microservices generating random letters and numbers
- [Servantes](https://github.com/windmilleng/servantes): a-little-bit-of-everything sample app with multiple microservices in different languages, showcasing many different Tilt behaviors
- [Frontend Demo](https://github.com/windmilleng/tilt-frontend-demo): Tilt + ReactJS
- [Live Update Examples](https://github.com/windmilleng/live_update): contains Go and Python examples of Tilt's [Live Update](https://docs.tilt.dev/live_update_tutorial.html) functionality
- [Sidecar Example](https://github.com/windmilleng/sidecar_example): simple Python app and home-rolled logging sidecar

### Go Modules

Currently, Tilt will not work with Go modules. See [this issue](https://github.com/windmilleng/tilt/issues/1520)
for more details.

If you're building Tilt from source, you must build it in your GOPATH.


## Performance
### Go Profile
We use the built-in Go profiler to debug performance issues.

When `tilt` is running, press `ctrl-p` to start the profile, and `ctrl-p` to stop it.
You should see output like:

```
starting pprof profile to tilt.profile
stopped pprof profile to tilt.profile
```

This means that Tilt has successfully written profiling data to the file `tilt.profile`.
In the directory where you ran Tilt, run:

```
go tool pprof tilt.profile
```

to open a special REPL that lets you explore the data.
Type `web` in the REPL to see a CPU graph.

For more information on pprof, see https://github.com/google/pprof/blob/master/doc/README.md.

### Opentracing
If you're trying to diagnose Tilt performance problems that lie between Tilt and your Kubernetes cluster (or between Tilt and Docker) traces can be helpful. The easiest way to get started with Tilt's [opentracing](https://opentracing.io/) support is to use the [Jaeger all-in-one image](https://www.jaegertracing.io/docs/1.11/getting-started/#all-in-one).

```
$ docker run -d --name jaeger \
  -e COLLECTOR_ZIPKIN_HTTP_PORT=9411 \
  -p 5775:5775/udp \
  -p 6831:6831/udp \
  -p 6832:6832/udp \
  -p 5778:5778 \
  -p 16686:16686 \
  -p 14268:14268 \
  -p 9411:9411 \
  jaegertracing/all-in-one:1.11
```

Then start Tilt with the following flags:

```
tilt up --trace --traceBackend jaeger
```

When Tilt starts one of the first lines in the log output should contain a trace ID, like so:

```
TraceID: 26256f1f6aa875e5
```

You can use the Jaeger UI (by default running on http://localhost:16686/) to query for this span and see all of the traces for the current Tilt run. These traces are made available immediately as you use Tilt. You don't need to wait until after Tilt has stopped to get access to the tracing data.

## Web UI

`tilt` uses a web interface for logs investigation.

By default, the web interface runs on port 10350.

When you use a released version of Tilt, all the HTML, CSS, and JS assets are served from our
[production bucket](https://console.cloud.google.com/storage/browser/tilt-static-assets).

When you build Tilt from head, the Tilt binary will default to development mode.
When you run Tilt, it will run a webpack dev server as a separate process on port 46764,
and reverse proxy all asset requests to the dev server.

To manually control the assets served, you can use:

```
tilt up --web-mode=local
```

to force Tilt to use the webpack dev server, or you can use

```
tilt up --web-mode=prod
```

to force Tilt to use production assets.


To run the server on an alternate port (e.g. 8001):

```
tilt up --port=8001
```

## Documentation

The landing page and documentation lives in
[the tilt.build repo](https://github.com/windmilleng/tilt.build/).

We write our docs in Markdown and generate static HTML with [Jekyll](https://jekyllrb.com/).

Netlify will automatically deploy the docs to [the public site](https://docs.tilt.dev/)
when you merge to master.

## Releasing

We use [goreleaser](https://goreleaser.com) for releases.

Requirements:
- goreleaser: `go get -u github.com/goreleaser/goreleaser`
- MacOS
- Python
- [gsutil](https://cloud.google.com/storage/docs/gsutil_install)
- `GITHUB_TOKEN` env variable with repo scope

Currently, releases have to be built on MacOS due to cross-compilation issues with Apple FSEvents.
Cross-compiling a Linux target binary with a MacOS toolchain works fine.

To create a new release at tag `$TAG`:

```
git fetch --tags
git tag -a v0.0.1 -m "my release"
git push origin v0.0.1
make release
```

goreleaser will build binaries for the latest tag (using semantic version to
determine "latest"). Check the current releases to figure out what the latest
release ought to be.

After updating the release notes,
update the [install](https://github.com/windmilleng/tilt.build/tree/master/docs/install.md) and [upgrade](https://github.com/windmilleng/tilt.build/blob/master/docs/upgrade.md) docs,
the [default dev version](internal/cli/build.go),
and the [installer version](scripts/install.sh).

### Version numbers
For pre-v1.0:
* If adding backwards-compatible functionality increment the patch version (0.x.Y).
* If adding backwards-incompatible functionality increment the minor version (0.X.y). We would probably **write a blog post** about this.

### Releasing the Synclet

Releasing a synclet should be very infrequent, because the amount of things it
does is small. (It's basically an optimization over `kubectl cp`, `kubectl
exec`, and restarting a container.)

To release a synclet, run `make synclet-release`. This will automatically:

- Publish a new synclet image tagged with the current date
- Update [sidecar.go](internal/synclet/sidecar/sidecar.go) with the new tag

Then submit the PR. The next time someone releases Tilt, it will use the new image tag.



