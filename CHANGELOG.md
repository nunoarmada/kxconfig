# Changelog

## [2.1.0](https://github.com/nunoarmada/kxconfig/compare/v2.0.1...v2.1.0) (2026-10-29)


### Features

* **kx:** set the default namespace when switching to a context without one ([#14](https://github.com/nunoarmada/kxconfig/issues/14)) ([574fc85](https://github.com/nunoarmada/kxconfig/commit/574fc8573461b87d497604cffef2b45beeb924eb))

## [2.0.1](https://github.com/nunoarmada/kxconfig/compare/v2.0.0...v2.0.1) (2026-10-28)


### Bug Fixes

* **kx:** activate the project when choosing one of its contexts in the TUI ([52da1fc](https://github.com/nunoarmada/kxconfig/commit/52da1fc3087429e82eded48b3b448d5ff618823c))
* **kx:** activate the project when choosing one of its contexts in the TUI ([460e092](https://github.com/nunoarmada/kxconfig/commit/460e092dd0355cb683c79cb964268c023db1c487))
* **kx:** activate the project when choosing one of its contexts in the TUI ([#12](https://github.com/nunoarmada/kxconfig/issues/12)) ([52da1fc](https://github.com/nunoarmada/kxconfig/commit/52da1fc3087429e82eded48b3b448d5ff618823c))

## [2.0.0](https://github.com/nunoarmada/kxconfig/compare/v1.6.0...v2.0.0) (2026-10-27)


### ⚠ BREAKING CHANGES

* replaces the kxconfig/kxswap bash scripts with the kx binary; module path is now github.com/nunoarmada/kxconfig/v2
* replaces the kxconfig/kxswap bash scripts with the kx binary; module path is now github.com/nunoarmada/kxconfig/v2

### Features

* **kx:** backup manager with retention ([1c1973a](https://github.com/nunoarmada/kxconfig/commit/1c1973af1479d29caa8888bd85be21cc73ae99f8))
* **kx:** cobra CLI ([d1c62dd](https://github.com/nunoarmada/kxconfig/commit/d1c62dd8a4c5aca6d917fdc4a4fd1b7d44157eca))
* **kx:** context deletion with orphan cleanup, ctx/ns setters ([02578d8](https://github.com/nunoarmada/kxconfig/commit/02578d8514883d55384001bc181214b3dd6262e5))
* **kx:** import/rename/delete/restore, target resolver, doctor, shell bookkeeping ([449fd7d](https://github.com/nunoarmada/kxconfig/commit/449fd7dff02ee214371f94aee2f8a533f171af1b))
* **kx:** kubeconfig load/save and conflict-aware merge ([9f83868](https://github.com/nunoarmada/kxconfig/commit/9f838686fe0c5de3e71a6ac44ed23a8922e31faf))
* **kx:** kx shell subshell with private kubeconfig copy ([6bd2c4b](https://github.com/nunoarmada/kxconfig/commit/6bd2c4b743bc5b1df6d01f5ae416418d2b076e49))
* **kx:** project store with symlink activation and safe adoption ([e58a06a](https://github.com/nunoarmada/kxconfig/commit/e58a06a9ca53b90baa8e6115b5b8de1d3a953d9e))
* **kx:** scaffold Go module and fsutil ([f5d87b2](https://github.com/nunoarmada/kxconfig/commit/f5d87b2fabf2d032aa9eb97b8e3f6093d5a59864))
* **kx:** TUI actions, dialogs, conflict resolution ([f679885](https://github.com/nunoarmada/kxconfig/commit/f679885467fcb27dfe7bd972471fa184f3f21650))
* **kx:** TUI main screen ([b0e9e1e](https://github.com/nunoarmada/kxconfig/commit/b0e9e1e5e50172544916b7fcf8589878a67c3d3c))
* rewrite kxconfig as the kx Go binary with a k9s-style TUI ([#10](https://github.com/nunoarmada/kxconfig/issues/10)) ([0edf5b6](https://github.com/nunoarmada/kxconfig/commit/0edf5b65837ab29965e95dd67d6c25a39cd239b7))
* ship kx Go binary and GoReleaser release ([c17e5e1](https://github.com/nunoarmada/kxconfig/commit/c17e5e177c4e6423474ec9e7920a7fba3eea1237))


### Bug Fixes

* **kx:** check close error in WriteFileAtomic ([4201b7a](https://github.com/nunoarmada/kxconfig/commit/4201b7a7d85ea5955b57df857f8ea216ca33487a))
* **kx:** confirm destructive CLI ops per review ([bb8bd9c](https://github.com/nunoarmada/kxconfig/commit/bb8bd9c318492feed9d8e2b2eac64d33fd9c340e))
* **kx:** confirm overwrites that re-point contexts, validate ns, warn on KUBECONFIG ([6810f50](https://github.com/nunoarmada/kxconfig/commit/6810f504055ff2c306d6521caa0f7aa9cf6ef815))
* **kx:** drop Unicode format characters, sanitize echoed project names ([74c3ee6](https://github.com/nunoarmada/kxconfig/commit/74c3ee60bc7035c411694b9623e47bc4bd53d27d))
* **kx:** harden merge against credential confusion; upgrade to Go 1.27 and client-go v0.37.1 ([aa0c489](https://github.com/nunoarmada/kxconfig/commit/aa0c4897d8bf79ee2803954c708924955981df17))
* **kx:** harden store ops and kx shell per review ([0cfa742](https://github.com/nunoarmada/kxconfig/commit/0cfa7422d7aadb0fc35ed7a137745651c0189e7a))
* **kx:** keep kx alive across shell signals and clean up ([9a7a34d](https://github.com/nunoarmada/kxconfig/commit/9a7a34dc75998d02e742d95370eaedcc6b251015))
* **kx:** lock active config, accurate KUBECONFIG warning, minor cleanups ([90bab7d](https://github.com/nunoarmada/kxconfig/commit/90bab7d9126975b218aa1861302fb50729a22f0e))
* **kx:** lock and back up exactly what is parsed, merge into a raw master ([985e272](https://github.com/nunoarmada/kxconfig/commit/985e272622ba5e96d341e2e744e01759cb7ceda3))
* **kx:** report rebound contexts, redact servers, merge from a raw master ([dd9313a](https://github.com/nunoarmada/kxconfig/commit/dd9313ab87ed88ef775490b184c86cbd4013a22a))
* **kx:** report the module version for go install builds ([ff70336](https://github.com/nunoarmada/kxconfig/commit/ff70336fa5c32b4fd340795375779880885ef094))
* **kx:** sanitize CLI confirmation prompts ([3952b47](https://github.com/nunoarmada/kxconfig/commit/3952b47b00efb7198c66a3dadacd1c8d7f83d31e))
* **kx:** sanitize kubeconfig-derived text before printing ([8d69b50](https://github.com/nunoarmada/kxconfig/commit/8d69b509cbf9dc137b18584cc777ad881cf2e64e))
* **kx:** TUI review and UX audit fixes ([1d06a28](https://github.com/nunoarmada/kxconfig/commit/1d06a2813c4b20238cae1cdf45fe9455e7c334fc))
* **kx:** validate backup op and keep backup on prune error ([ad56bbc](https://github.com/nunoarmada/kxconfig/commit/ad56bbc414d31fd0a9e2c88da2faf677516bd2a2))

## [1.6.0](https://github.com/nunoarmada/kxconfig/compare/kxconfig-v1.5.1...kxconfig-v1.6.0) (2025-12-04)


### Features

* **kxconfig:** Enhance context deletion handling with backup and active config update ([b2b43b0](https://github.com/nunoarmada/kxconfig/commit/b2b43b0a795641a78e195edd1a3fc9f79cce7e3c))

## [1.5.1](https://github.com/nunoarmada/kxconfig/compare/kxconfig-v1.5.0...kxconfig-v1.5.1) (2025-12-04)


### Bug Fixes

* **kxconfig:** Update context deletion commands to use --kubeconfig flag for improved clarity and error handling ([2b07554](https://github.com/nunoarmada/kxconfig/commit/2b075548928adcbdc0d27d485da3574391ffdaa9))

## [1.5.0](https://github.com/nunoarmada/kxconfig/compare/kxconfig-v1.4.2...kxconfig-v1.5.0) (2025-11-26)


### Features

* Add project name validation and sorting in kxconfig and kxswap scripts ([8362077](https://github.com/nunoarmada/kxconfig/commit/8362077820422ae1ad85c514547cc5232de47975))

## [1.4.2](https://github.com/nunoarmada/kxconfig/compare/kxconfig-v1.4.1...kxconfig-v1.4.2) (2025-11-26)


### Bug Fixes

* Redirect terminal output to /dev/tty in kxswap script ([22c849f](https://github.com/nunoarmada/kxconfig/commit/22c849fdce7f2825b5858e0f85e35b1b1e5b3260))

## [1.4.1](https://github.com/nunoarmada/kxconfig/compare/kxconfig-v1.4.0...kxconfig-v1.4.1) (2025-11-26)


### Bug Fixes

* Improve tag retrieval in Homebrew update workflow ([d23daf6](https://github.com/nunoarmada/kxconfig/commit/d23daf6db273493243747c99b7f75284be1f860f))

## [1.4.0](https://github.com/nunoarmada/kxconfig/compare/kxconfig-v1.3.0...kxconfig-v1.4.0) (2025-11-26)


### Features

* Enhance kxswap script with interactive project selection ([606194f](https://github.com/nunoarmada/kxconfig/commit/606194fe286f19b2aa459267ba355e98acca3d2a))
* Update installation methods and add Homebrew support ([93a9c02](https://github.com/nunoarmada/kxconfig/commit/93a9c02d471b2f499e0ac2d7d27434863451457b))

## [1.3.0](https://github.com/nunoarmada/kxconfig/compare/kxconfig-v1.2.0...kxconfig-v1.3.0) (2025-11-26)


### Features

* Add completion script installation to install.sh and implement kubectx-config completion functionality ([4a128a3](https://github.com/nunoarmada/kxconfig/commit/4a128a3eaa25672eb6b1f458cd087f3d10ffcc75))
* Add contributing guide and CI workflows for linting and testing ([f88fd02](https://github.com/nunoarmada/kxconfig/commit/f88fd028aab00425f08d8d9c53d47c79636e6aef))
* Enhance kxconfig to backup current kubeconfig on first usage and update documentation ([9c868f2](https://github.com/nunoarmada/kxconfig/commit/9c868f27a342e3c62c2f84aa04e263adbdef6514))
* Implement CI workflows for linting and testing scripts ([f81462f](https://github.com/nunoarmada/kxconfig/commit/f81462fd60d8ce356292dd37032263fecff6500b))
* Introduce kxconfig and kxswap scripts for kubeconfig management and enhance installation process ([4a26b38](https://github.com/nunoarmada/kxconfig/commit/4a26b384d25118e45029dad90e40e1d9278a1eda))

## [1.2.0](https://github.com/nunoarmada/kubectx-config/compare/kubectx-config-v1.1.0...kubectx-config-v1.2.0) (2025-11-26)


### Features

* Add contributing guide and CI workflows for linting and testing ([f88fd02](https://github.com/nunoarmada/kubectx-config/commit/f88fd028aab00425f08d8d9c53d47c79636e6aef))
* Implement CI workflows for linting and testing scripts ([f81462f](https://github.com/nunoarmada/kubectx-config/commit/f81462fd60d8ce356292dd37032263fecff6500b))

## [1.1.0](https://github.com/nunoarmada/kubectx-config/compare/kubectx-config-v1.0.0...kubectx-config-v1.1.0) (2025-11-19)


### Features

* Add completion script installation to install.sh and implement kubectx-config completion functionality ([4a128a3](https://github.com/nunoarmada/kubectx-config/commit/4a128a3eaa25672eb6b1f458cd087f3d10ffcc75))
* Enhance kxconfig to backup current kubeconfig on first usage and update documentation ([9c868f2](https://github.com/nunoarmada/kubectx-config/commit/9c868f27a342e3c62c2f84aa04e263adbdef6514))
* Introduce kxconfig and kxswap scripts for kubeconfig management and enhance installation process ([4a26b38](https://github.com/nunoarmada/kubectx-config/commit/4a26b384d25118e45029dad90e40e1d9278a1eda))
