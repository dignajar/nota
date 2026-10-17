# Security policy

## Supported versions

Only the latest release receives security fixes. nota ships a release on every
merge to `main`, so upgrading is one command:

```sh
curl -fsSL https://raw.githubusercontent.com/dignajar/nota/main/install.sh | sh
```

## Reporting a vulnerability

Please do not open a public issue. Report it privately through
[GitHub security advisories](https://github.com/dignajar/nota/security/advisories/new).

Include the nota version (`nota --version`), your OS, and the steps to
reproduce. You can expect a first answer within a week. Once a fix is
released, the advisory is published and you are credited unless you prefer
otherwise.

## Verifying a release

Every release is built by GitHub Actions. Its `checksums.txt` is signed with
keyless cosign and has a build provenance attestation:

```sh
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/dignajar/nota/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify nota_<version>_<os>_<arch>.tar.gz --repo dignajar/nota
```
