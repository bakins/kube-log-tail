# kube-log-tail

tail/follow logs from multiple pods and their containers.

`kube-log-tail` will follow logs of new pods that are created while it
is running.

Inspired by [kubetail](https://github.com/johanhaleby/kubetail).

## Installation

To install the latest master, you need a working local Go environment,
and run:

```shell
go get -u github.com/bakins/kube-log-tail/cmd/kube-log-tail
```

For released versions, see [Releases](https://github.com/bakins/kube-log-tail/releases). Download, chmod +x the file,
and move it into your PATH.

## Usage

```shell
$ kube-log-tail -h
tail kubernetes pod logs

Usage:
  kube-log-tail [flags]

Flags:
  -k, --colored-output string   use colored output (pod|line|off) (default "pod")
  -h, --help                    help for kube-log-tail
      --kubeconfig string       path to kubeconfig (default "/Users/bakins/.kube/config")
  -n, --namespace string        namespace for pods. use "" for all namespaces (default "default")
  -r, --refresh duration        how often to refresh the list of pods (default 10s)
  -l, --selector string         label selector for pods
  -v, --version                 display the current version
  ```

The selector is a [label selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors). Simple selectors such as `app=my-app` as well as more complex selectors
such as `environment in (production, qa)` are supported.

Namespace limits the pod search to specific namespaces. Use an empty namespace (ie, `--namespace=""`) to search all namespaces.

`kube-log-tail` will refresh the list of pods periodically. One may change the interval, by passing in `--refesh=1m`, for example, to only refresh once per minute. Deleted pods will no longer output, and new pods will begin tailing without having to restart `kube-log-tail`.

By using the `-k` argument you can specifiy how `kube-log-tail` makes use of colors.

| Value   |     Description  |
|----------|---------------|
| pod | Only the pod name is colorized but the logged text is using the terminal default color  (default)|
| line | The entire line is colorized |
| false | Don't colorize the output at all |

Only a small number of colors are currently supported, and each container in each pod
is given a color, so colors may repeat.

## Know Issues

- Sometimes, `kube-log-tail` will just stop tailing a log. If this happens, check if the last log line from the container was `unexpected stream type ""` or similar. This is an [issue](https://github.com/kubernetes/kubernetes/issues/47800) with Kubernetes and I'm thinking of workaround.

## TODO

- [Homebrew](https://brew.sh/) recipe
- use watches for pod events rather than a full refresh
- allow disabling seeing old log lines
- Handle containers that sre not running - ie, in ContainerStart

## LICENSE

See [LICENSE](./LICENSE)

## Acknowledgements

Thanks to [Johan Haleby](https://github.com/johanhaleby) for kubetail.