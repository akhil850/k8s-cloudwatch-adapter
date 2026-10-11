# Contributing

These guidelines have been rewritten for this independently maintained fork.

## Bugs and feature requests

Search this fork's [issues](https://github.com/akhil850/k8s-cloudwatch-adapter/issues) before opening a new report. Include the image tag or commit, Kubernetes version, relevant metric configuration, steps to reproduce, and expected versus actual behavior. Remove credentials and sensitive information from logs and examples.

See [ROADMAP.md](ROADMAP.md) for planned work. Discuss significant changes in an issue before implementing them.

## Pull requests

1. Create a branch from `master` in your fork.
2. Keep changes focused and preserve the distinction between missing CloudWatch data and a valid zero measurement.
3. Add regression tests for behavior changes and run `go test ./pkg/...`.
4. Open a pull request against this repository's `master` branch with a description of the change and its validation.
5. Address review feedback and CI failures.

## Conduct

Follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Project discussions and reports are handled by this fork's maintainer.

## Security reports

Do not disclose vulnerabilities, credentials, or exploit details in public issues. Use GitHub's private vulnerability reporting feature if it is enabled for this repository. Otherwise, contact the [maintainer](https://github.com/akhil850) using a contact method listed on their profile. If no private channel is available, request one without posting vulnerability details.

## License

Contributions are submitted under the project's [Apache License 2.0](LICENSE). Preserve applicable copyright and attribution notices, including [NOTICE](NOTICE), and identify modifications to upstream files.
