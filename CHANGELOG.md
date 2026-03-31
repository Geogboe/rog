# Changelog

## [0.5.0](https://github.com/Geogboe/rog/compare/v0.4.1...v0.5.0) (2026-03-31)


### Features

* **update:** add rog update self-update command ([3c84c91](https://github.com/Geogboe/rog/commit/3c84c9119cfa83de61dfda87f57a1d11adc4f327))

## [0.4.1](https://github.com/Geogboe/rog/compare/v0.4.0...v0.4.1) (2026-03-31)


### Bug Fixes

* **config:** convert --edit flag to config edit subcommand ([#25](https://github.com/Geogboe/rog/issues/25)) ([5dec1f7](https://github.com/Geogboe/rog/commit/5dec1f7dfe944b04bf8df6422e9778d0d960ca9a))
* **install:** redirect info/ok output to stderr ([475b61f](https://github.com/Geogboe/rog/commit/475b61f8f01a17558cb6ce6ee9f1d0f9a2d5c927)), closes [#35](https://github.com/Geogboe/rog/issues/35)

## [0.4.0](https://github.com/Geogboe/rog/compare/v0.3.0...v0.4.0) (2026-03-25)


### Features

* improve scan progress and windows path handling ([c60522d](https://github.com/Geogboe/rog/commit/c60522dce208c68f5c3763a8dbb4d3834bcf2164))

## [0.3.0](https://github.com/Geogboe/rog/compare/v0.2.0...v0.3.0) (2026-03-24)


### Features

* **version:** add version command to display installed version ([03a079e](https://github.com/Geogboe/rog/commit/03a079eebace4dca6bd7fc9310f814b585a21fd9))

## [0.2.0](https://github.com/Geogboe/rog/compare/v0.1.0...v0.2.0) (2026-03-24)


### Features

* **install:** add install scripts for Linux/macOS and Windows ([9c92713](https://github.com/Geogboe/rog/commit/9c9271342f286e6f27af186fea3e4512f00a441f))

## 0.1.0 (2026-02-04)


### Features

* add GitHub releases pipeline with multi-platform builds ([2aa09be](https://github.com/Geogboe/rog/commit/2aa09beb82040ccbec67a2104994081b18c0761e))
* add WSL support infrastructure ([e6c82b9](https://github.com/Geogboe/rog/commit/e6c82b9f65873674482bbc4fdc3e8c3abe0f6158))
* **cli:** add implicit list command alias ([6da707c](https://github.com/Geogboe/rog/commit/6da707ca39132163a14352c8b3183d2edc3b2e17))
* **cli:** add shell completion support and verbose/debug logging ([613727e](https://github.com/Geogboe/rog/commit/613727ed8b371f8b9a03a7cfe7564db5e3d0aa9c))
* implement all CLI commands and fix build errors ([5960aa5](https://github.com/Geogboe/rog/commit/5960aa56ceccc31c015005866ce75411693d177d))
* implement core packages (config, index, git, metadata, scanner) ([2a288b9](https://github.com/Geogboe/rog/commit/2a288b94221c329821afd520576456a600210f46))
* **list:** add --fields flag for custom column selection ([65f3389](https://github.com/Geogboe/rog/commit/65f3389e0df66db4593a1216bc185eeb1873078e))
* **list:** display descriptions in default and long output modes ([679aba5](https://github.com/Geogboe/rog/commit/679aba5eccce73957c65f34ee94aced2a356a5be))
* **list:** display descriptions in default and long output modes ([a98da6c](https://github.com/Geogboe/rog/commit/a98da6ca4603a0a3da3c5f04c417f12488081176))
* **list:** move description to --long only and add -s/-l aliases ([48d7499](https://github.com/Geogboe/rog/commit/48d7499daf0729f15fdc350bc69560c65ed4e59f))
* **scan:** add global excludes, parallel roots, debug logging, and config validation ([c0b7202](https://github.com/Geogboe/rog/commit/c0b720284cec0c140939f5d445640a454f4d92c1))
* **scan:** add README description parsing and improve flag UX ([8a63498](https://github.com/Geogboe/rog/commit/8a6349802b3f5c8dfe1f74f9de8135ee1383b177))
* **select:** add description display and --open flag ([3c377f8](https://github.com/Geogboe/rog/commit/3c377f8d34054612276fc3946a337efcbbd7dc1e))


### Bug Fixes

* add explicit permissions to test workflow for security ([3f1e372](https://github.com/Geogboe/rog/commit/3f1e3727ffe97140591ead691a7cce9a966736ad))
* **cli:** handle global flags with implicit list alias ([ea4359e](https://github.com/Geogboe/rog/commit/ea4359e95d85432fbb2f224abf21d647b7f7ad21))
* **completion:** make completion command visible in help output ([74fded5](https://github.com/Geogboe/rog/commit/74fded5e333dba3378a8524f4fbcc8dfd59875f9))
* **scanner:** prevent Makefile from overriding Python detection ([5905508](https://github.com/Geogboe/rog/commit/5905508ee16322e729886f62f642ebc0ad8154d2))
* **scanner:** properly handle nested roots with longest path matching ([c5083cd](https://github.com/Geogboe/rog/commit/c5083cd1cda2b08a8bc14ccfed620c5d6fe32100))
* **scanner:** properly handle nested roots with longest path matching ([fbffea7](https://github.com/Geogboe/rog/commit/fbffea7e96483f6469d8ac19443e58501705c72e))
* **select:** align columns with fixed-width formatting ([7f5f849](https://github.com/Geogboe/rog/commit/7f5f849232b1302240b592984f6d2b23a27f7630))


### Performance Improvements

* **scan:** implement parallel directory walking and dry-run metrics ([8459ae6](https://github.com/Geogboe/rog/commit/8459ae6441b55bab345cf334fdff2deef0ef738a))
* **scan:** implement parallel directory walking and dry-run metrics ([5046fee](https://github.com/Geogboe/rog/commit/5046fee9e43265659e0966766c6568e289258b02))
* **scan:** integrate fd for fast repo discovery and optimize git operations ([54ae0bc](https://github.com/Geogboe/rog/commit/54ae0bcd851592ee42580e0e6345036afeac3c90))
