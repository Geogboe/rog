# Changelog

## [0.7.0](https://github.com/Geogboe/rog/compare/v0.6.1...v0.7.0) (2026-09-30)


### Features

* **list:** add compact color table and output flag ([241bcdf](https://github.com/Geogboe/rog/commit/241bcdf6ae821e6f010283ef57c4dcb31c5938b8))
* **report:** add work reports across configured roots ([9f5ee68](https://github.com/Geogboe/rog/commit/9f5ee686a97719caeb8763a76194393a66a13f73))
* **report:** show live collection and AI progress ([49915c3](https://github.com/Geogboe/rog/commit/49915c3a47d5ddd05950df843dd0355b9cb71df9))
* **scan:** add native discovery, WSL worker, and picker ([0907312](https://github.com/Geogboe/rog/commit/09073128318325231f0db4cbcd39bcf7f35e0833))
* **scan:** show repository name in live progress ([064a404](https://github.com/Geogboe/rog/commit/064a404c641b400d48be4d8f47aa5357be85a8eb))
* **setup:** add guided configuration wizard ([c3ee58a](https://github.com/Geogboe/rog/commit/c3ee58a8b22545bbac1d9607ed29389bb28d4b60))
* **ui:** add restrained color to picker and info ([84e20cd](https://github.com/Geogboe/rog/commit/84e20cd169de407156506feaa2afa969c71ae73f))
* **wsl:** scan Windows roots with native worker ([7bb3d9c](https://github.com/Geogboe/rog/commit/7bb3d9c904c91b881493d0b8ef5515e9a79f2f09))


### Bug Fixes

* **git:** avoid optional index locks during status ([79b6646](https://github.com/Geogboe/rog/commit/79b66466f57e3e446c67e5dbee5433ff658661ab))
* **git:** hide Windows scan subprocesses ([97d062c](https://github.com/Geogboe/rog/commit/97d062cb08c1607724c94c83e881aef58ae828b3))
* **list:** align colored columns to terminal width ([049e00a](https://github.com/Geogboe/rog/commit/049e00ab7ccb3350fd7d94f3d7a48e7b3c88f366))
* **report:** honor Ctrl+C during AI confirmation ([0ff8b28](https://github.com/Geogboe/rog/commit/0ff8b2873273997f527c642fee0375ba28b3dec2))
* **report:** include safe current filenames in AI evidence ([b0905e6](https://github.com/Geogboe/rog/commit/b0905e6652bf0ae76cd896cde98348a2cb814c24))
* **report:** open generated HTML from a directory ([77af38b](https://github.com/Geogboe/rog/commit/77af38bc55cae9afeb8a15355fb0fc57a2939c89))
* **scan:** handle mixed Windows and WSL catalogs safely ([3dc0de7](https://github.com/Geogboe/rog/commit/3dc0de782e24a482b2f353484010e247853dd058))
* **setup:** count native Windows repositories in coverage ([20f257a](https://github.com/Geogboe/rog/commit/20f257a22574c51ecd49e14287403e802405ac3b))
* **setup:** preserve config extensions and add Windows locations ([18262e4](https://github.com/Geogboe/rog/commit/18262e44e46585ca12aa57314996fa22114ad606))
* **ui:** keep picker details next to short results ([002c624](https://github.com/Geogboe/rog/commit/002c6247e96f3da3bdf0ef79f9e97bb44fd466df))
* **wsl:** hide Windows worker launches ([3426a69](https://github.com/Geogboe/rog/commit/3426a69ca9dbf1af9a08c50b9125d91b63267515))


### Performance Improvements

* **report:** batch commit change stats ([ef10fa4](https://github.com/Geogboe/rog/commit/ef10fa417b78a6946d14c31fbc9bc9b176a90091))
* **report:** reduce Git launches during repository grouping ([97073b0](https://github.com/Geogboe/rog/commit/97073b07f23ed457f62ed709ad0ec61defbb8e0f))
* **scan:** reuse indexed metadata by default ([5bbc068](https://github.com/Geogboe/rog/commit/5bbc0684ccd917d1e78117c62c155ff96fb757d1))
* **scan:** speed mixed Windows and WSL scans ([836032c](https://github.com/Geogboe/rog/commit/836032cd6ddadb9586e9431134b66662735879cb))

## [0.6.1](https://github.com/Geogboe/rog/compare/v0.6.0...v0.6.1) (2026-08-12)


### Bug Fixes

* **selfupdate:** address rog[#48](https://github.com/Geogboe/rog/issues/48) Copilot review findings ([871c3af](https://github.com/Geogboe/rog/commit/871c3afa76f8334188f792f1678639a5fc508d88))
* **selfupdate:** address rog[#48](https://github.com/Geogboe/rog/issues/48) Copilot review findings ([0b3adc4](https://github.com/Geogboe/rog/commit/0b3adc4217aa55ecb9ffa26b2654bb3f78e389a5))

## [0.6.0](https://github.com/Geogboe/rog/compare/v0.5.0...v0.6.0) (2026-08-12)


### Features

* **selfupdate:** support fetching newest release including prereleases ([f057726](https://github.com/Geogboe/rog/commit/f057726be9b1b2d62c6dc27663dcfd793a893d63))
* **selfupdate:** support fetching newest release including prereleases ([4058b05](https://github.com/Geogboe/rog/commit/4058b05684ff6c48199411a1798ae485ad7f2feb))


### Performance Improvements

* **scanner:** eliminate subprocess spawns via direct .git filesystem reads ([3d121f5](https://github.com/Geogboe/rog/commit/3d121f5ab4e00b35be73c70ddfa28fa9ffe8e3c4))
* **scanner:** replace subprocess spawns with direct filesystem reads ([f5b58a9](https://github.com/Geogboe/rog/commit/f5b58a9aa3f7ae0d778f13ec75743faa7a6ca79b))

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
