# Changelog

## [1.0.3](https://github.com/skillrig/cli/compare/v1.0.2...v1.0.3) (2026-06-05)


### Bug Fixes

* **fetch:** resolve catalog for a non-default-branch origin ([#32](https://github.com/skillrig/cli/issues/32)) ([3b97e80](https://github.com/skillrig/cli/commit/3b97e8018001ec491043d9e0ffbce8d7c7c12134))

## [1.0.2](https://github.com/skillrig/cli/compare/v1.0.1...v1.0.2) (2026-06-04)


### Bug Fixes

* **auth:** force git non-interactive on private-origin fetch ([#30](https://github.com/skillrig/cli/issues/30)) ([3456441](https://github.com/skillrig/cli/commit/3456441fc7d7d58ca5d6329a85f5410887197e09))

## [1.0.1](https://github.com/skillrig/cli/compare/v1.0.0...v1.0.1) (2026-06-02)


### Bug Fixes

* add MIT license ([#14](https://github.com/skillrig/cli/issues/14)) ([346bce0](https://github.com/skillrig/cli/commit/346bce0465fbaa058c4af82a4091e1dd67a6bc59))
* **cli:** align search & verify table output ([#21](https://github.com/skillrig/cli/issues/21)) ([#22](https://github.com/skillrig/cli/issues/22)) ([b54d751](https://github.com/skillrig/cli/commit/b54d751963560b60e910b34d9ed9fb621c540716))

## 1.0.0 (2026-06-01)


### Features

* **001:** implement skillrig init + single origin resolver ([1e9beda](https://github.com/skillrig/cli/commit/1e9beda6d54f828c754deb6aab6f70ad0dd4cf78))
* **001:** skillrig init + single origin resolver ([ba98d06](https://github.com/skillrig/cli/commit/ba98d06bf528b7a34070608c739e8c22c45eed1a))
* **001:** support optional branch/ref in origin reference ([b6468e0](https://github.com/skillrig/cli/commit/b6468e05178bf71e4e888ef4e575f5ba8f77a8f5))
* **001:** support optional branch/ref in origin reference (OWNER/REPO[@REF]) ([dc7924c](https://github.com/skillrig/cli/commit/dc7924c83ba66420f8caa6005a96039121564ee9))
* **002:** skillcore SDK + add/verify (vendor & verify skills) ([168afd1](https://github.com/skillrig/cli/commit/168afd110bdcb29859481ff9a2bfacf71c3cab3a))
* **002:** vendor & verify skills — skillcore SDK + add/verify ([99eb7ee](https://github.com/skillrig/cli/commit/99eb7ee8ed4859be602416271f83592484ced301))
* **003:** Discover & Acquire — search + remote add + index (manifest→frontmatter) ([48afc69](https://github.com/skillrig/cli/commit/48afc69def245e950d60daeb9a62741da4e686e7))
* **003:** implement search + remote add + index (manifest→frontmatter) ([429f0ca](https://github.com/skillrig/cli/commit/429f0ca3f291d35460a949140f1f0b8099858793))


### Bug Fixes

* **001:** address Qodo review — surface swallowed errors, fix zero-origin String, scope todo rule ([d95b830](https://github.com/skillrig/cli/commit/d95b830fc654a1ee26d8a84567697fe393f85921))
* **001:** surface resolver source diagnostics (FR-004) + close review gaps ([3910538](https://github.com/skillrig/cli/commit/39105381a01dde876555d6e26e3f7ec9bc367b2e))
* **002:** address Qodo PR review — security + correctness hardening ([76b560e](https://github.com/skillrig/cli/commit/76b560efcc3169321d2fcb6dc6b7f8e631632956))
* **002:** resolve adversarial review [#2](https://github.com/skillrig/cli/issues/2) findings ([4b0e33a](https://github.com/skillrig/cli/commit/4b0e33a0e618b09f504a5c4fa558d3a41177773b))
* **003:** address Qodo PR[#8](https://github.com/skillrig/cli/issues/8) — search git-repo/Args, token via GIT_CONFIG env ([d836344](https://github.com/skillrig/cli/commit/d836344a05dd09547e902ce80b633edec6819da2))
* **003:** remediate adversarial deep-dive — remote keystone now real + tested ([5f90a82](https://github.com/skillrig/cli/commit/5f90a821eb340ba8772354b73770b28759b3cc69))
