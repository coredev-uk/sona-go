# Changelog

## [0.7.0](https://github.com/coredev-uk/sona-go/compare/v0.6.0...v0.7.0) (2026-11-18)


### Features

* **addons:** a WebAssembly host for add-ons ([#547](https://github.com/coredev-uk/sona-go/issues/547)) ([522b7e5](https://github.com/coredev-uk/sona-go/commit/522b7e50b029b772c37c755f30ad22ce7cc3dd69))
* **addons:** Discord presence runs as the bundled discord add-on ([#559](https://github.com/coredev-uk/sona-go/issues/559)) ([ee2f71e](https://github.com/coredev-uk/sona-go/commit/ee2f71eae36598d9ba13e0bdfe6cc0b1a1513987))
* **addons:** Last.fm runs on the add-on API, with its browser approval ([#558](https://github.com/coredev-uk/sona-go/issues/558)) ([f24365f](https://github.com/coredev-uk/sona-go/commit/f24365f10e052d396c1f3532d026ec9f1b6a1586))
* **addons:** ListenBrainz runs on the add-on API, behind Sona's queue ([#555](https://github.com/coredev-uk/sona-go/issues/555)) ([6b2d4e6](https://github.com/coredev-uk/sona-go/commit/6b2d4e629b82b8538d0675414216b04c9bef1172))
* **addons:** OS sandbox for out-of-process add-ons ([#552](https://github.com/coredev-uk/sona-go/issues/552)) ([384260a](https://github.com/coredev-uk/sona-go/commit/384260a84a1fdec1ffed3f6ff0d22b2d5c6fad80))
* **addons:** sign-ins move to the add-ons' keyring items, and build credentials come in hello ([#551](https://github.com/coredev-uk/sona-go/issues/551)) ([fd2f536](https://github.com/coredev-uk/sona-go/commit/fd2f5360043809fd47b36920fc36133665d9a20e))
* **addons:** Sona's own add-ons are registered in code and run as sona --addon &lt;id&gt; ([#545](https://github.com/coredev-uk/sona-go/issues/545)) ([7b1a7ac](https://github.com/coredev-uk/sona-go/commit/7b1a7acf83cd2222332b092779b7e89f476eaf59))
* **addons:** the network permission ([#542](https://github.com/coredev-uk/sona-go/issues/542)) ([c205abd](https://github.com/coredev-uk/sona-go/commit/c205abd56011ef4a2b83817f7c8a75e213eee060))
* **addons:** UI contributions, Settings → Add-ons and sona addons dev ([#502](https://github.com/coredev-uk/sona-go/issues/502)) ([4a25ae8](https://github.com/coredev-uk/sona-go/commit/4a25ae8005376aebbe37f1862d77aa0044158852))
* **cli:** sona connect: the add-on protocol on stdio for your own programs ([#553](https://github.com/coredev-uk/sona-go/issues/553)) ([30fed2e](https://github.com/coredev-uk/sona-go/commit/30fed2e0c28a5bd0f3b8f57159240102462f6a01))
* **cli:** stream deck controls: seek, shuffle, repeat, crossfade, queue, dislike, library, play links ([#544](https://github.com/coredev-uk/sona-go/issues/544)) ([c1b5078](https://github.com/coredev-uk/sona-go/commit/c1b5078571a69f08e8b7e0cc141b977a7fa4d346))
* **library:** sync music videos into SQLite so the page works offline ([#516](https://github.com/coredev-uk/sona-go/issues/516)) ([952368a](https://github.com/coredev-uk/sona-go/commit/952368ad95c960a28dead7d7ee77b38fc93d54c7))
* **lyrics:** Full-Screen Lyrics: a phrasing cascade for TVs and second screens ([#569](https://github.com/coredev-uk/sona-go/issues/569)) ([3f323dc](https://github.com/coredev-uk/sona-go/commit/3f323dc28656a7d21da91640a5b915e79e0298e5))
* **playback:** play Spatial Audio's binaural rendition in playspike; fmp4 reads each fragment's sample entry ([#549](https://github.com/coredev-uk/sona-go/issues/549)) ([8f4a4df](https://github.com/coredev-uk/sona-go/commit/8f4a4dfa9c871a2a00fb3c370c703ece71110fd5))
* **playback:** play Spatial Audio's binaural rendition in the engine ([#557](https://github.com/coredev-uk/sona-go/issues/557)) ([eb1fb6b](https://github.com/coredev-uk/sona-go/commit/eb1fb6b48b8d9e101d3c596a69ec2cefa1089a2b))
* **playback:** Spatial Audio applies at once, and says why a song isn't spatial ([#566](https://github.com/coredev-uk/sona-go/issues/566)) ([6d30e6d](https://github.com/coredev-uk/sona-go/commit/6d30e6d9635f523787d5e42414523527a34a110c))
* **settings:** Add-ons is a roster that holds many add-ons ([#565](https://github.com/coredev-uk/sona-go/issues/565)) ([c8e0a9a](https://github.com/coredev-uk/sona-go/commit/c8e0a9afe8ffc4a991ceb4f21dca818bf1c87162))
* **settings:** Developer tools: Inspect in release builds for add-on developers ([#567](https://github.com/coredev-uk/sona-go/issues/567)) ([c2a61f0](https://github.com/coredev-uk/sona-go/commit/c2a61f0ccadc868a9a5d986a0e4c57265702c6a6))
* **settings:** Inspect panel: ledger, tabs and a JSON tree ([#568](https://github.com/coredev-uk/sona-go/issues/568)) ([6e8df68](https://github.com/coredev-uk/sona-go/commit/6e8df68535a2cb16a5d69daae11dc12762d40cad))
* **ui:** a modified click selects where the list selects and opens where it cannot ([#510](https://github.com/coredev-uk/sona-go/issues/510)) ([994d109](https://github.com/coredev-uk/sona-go/commit/994d109bc7d59970d98e248ca8fd8dbe38c03411))
* **ui:** draw a shuffle glyph where a sidebar row shuffles and a cue on Play/Shuffle pins ([#497](https://github.com/coredev-uk/sona-go/issues/497)) ([f64da92](https://github.com/coredev-uk/sona-go/commit/f64da92cbd17e7540e60dfc4dc57ffd84af750af))
* **ui:** Play disc on local shelves, library grids and artist release rows ([#511](https://github.com/coredev-uk/sona-go/issues/511)) ([5e84c47](https://github.com/coredev-uk/sona-go/commit/5e84c4742bda0445a115e4158dcca8419d903d11))
* **ui:** right-click opens the menu on more surfaces ([#520](https://github.com/coredev-uk/sona-go/issues/520)) ([1813734](https://github.com/coredev-uk/sona-go/commit/18137343a925b3da83a9605ac12525044980a0b7))
* **ui:** Shift+Enter on a suggestion is Enter ([#500](https://github.com/coredev-uk/sona-go/issues/500)) ([6a03874](https://github.com/coredev-uk/sona-go/commit/6a03874394c2f5752916a832f34630e13744ddb8))


### Bug Fixes

* **addons:** Sona's own add-ons run without the sandbox where it can't start ([#561](https://github.com/coredev-uk/sona-go/issues/561)) ([2087936](https://github.com/coredev-uk/sona-go/commit/2087936fb7311a9453554fa4f347045400f6555c))
* **addons:** Sona's own add-ons start on NixOS and are never switched off by the crash rule ([#564](https://github.com/coredev-uk/sona-go/issues/564)) ([0bda240](https://github.com/coredev-uk/sona-go/commit/0bda240016b4dd258b17735ab4632e427be2b041))
* **album:** fit the Play disc to the sleeve and the equaliser to the ground at 3:1 ([#530](https://github.com/coredev-uk/sona-go/issues/530)) ([fac2ae9](https://github.com/coredev-uk/sona-go/commit/fac2ae922ed8bd9c85adae89a8391e8f0b862fb5))
* **artwork:** playlist photos chosen in Apple's apps show ([#562](https://github.com/coredev-uk/sona-go/issues/562)) ([e333c5f](https://github.com/coredev-uk/sona-go/commit/e333c5fe5be04517377c8a569c826ac143b76ede))
* **db:** renumber the recent-searches migration to 00017 ([#533](https://github.com/coredev-uk/sona-go/issues/533)) ([82f9ac8](https://github.com/coredev-uk/sona-go/commit/82f9ac896e836ab4a28612b5dcbc2b750362d044))
* **home:** the lone hero is a colour band with the station's square art ([#537](https://github.com/coredev-uk/sona-go/issues/537)) ([45da4c4](https://github.com/coredev-uk/sona-go/commit/45da4c49bad8ef4e66e65d5bc8c19b433e18e052))
* **home:** the lone hero's banner fills the row beside its text ([#536](https://github.com/coredev-uk/sona-go/issues/536)) ([da277ff](https://github.com/coredev-uk/sona-go/commit/da277ff5fba88c70786dae48428734e83f12c714))
* **lyrics:** nothing of the lyrics shows behind Full-Screen Lyrics' strip ([#570](https://github.com/coredev-uk/sona-go/issues/570)) ([5844fde](https://github.com/coredev-uk/sona-go/commit/5844fde8ccb5e625dd26219e59904dd44174642f))
* **player:** a pause is not undone by an older engine event, and the flaky player tests wait for what they use ([#574](https://github.com/coredev-uk/sona-go/issues/574)) ([8443b46](https://github.com/coredev-uk/sona-go/commit/8443b461f3b5fa2aaaa2cc4e783dd3d9c4c579bc))
* **playlist:** name Play and Shuffle for the songs found while Find is active ([#529](https://github.com/coredev-uk/sona-go/issues/529)) ([4c368ca](https://github.com/coredev-uk/sona-go/commit/4c368cace3563c063ba2342fc4941e061d7ce81c))
* **radio:** start a station song's several artists under its name ([#528](https://github.com/coredev-uk/sona-go/issues/528)) ([ed311cb](https://github.com/coredev-uk/sona-go/commit/ed311cbd19f332518233da54d4c651e9adcc1b30))
* **services:** mark library-only songs Apple withholds as unavailable ([#532](https://github.com/coredev-uk/sona-go/issues/532)) ([a1e263f](https://github.com/coredev-uk/sona-go/commit/a1e263ffce36b94dac9a6196af475ed26ed146bd))
* **settings:** the page lays out in its own width, not the window's ([#548](https://github.com/coredev-uk/sona-go/issues/548)) ([605b187](https://github.com/coredev-uk/sona-go/commit/605b1879338ba0161c056f2df947de8b7373a786))
* **ui:** artists and curators open only; Home's lead pick draws no dead Play ([#512](https://github.com/coredev-uk/sona-go/issues/512)) ([4334797](https://github.com/coredev-uk/sona-go/commit/433479792cfd0fd6b831bf486916bdeac753573c))
* **ui:** items whose page is not routed are plain on cards, link tiles and the artist slip ([#509](https://github.com/coredev-uk/sona-go/issues/509)) ([d1279c1](https://github.com/coredev-uk/sona-go/commit/d1279c1209bac1c08b6c1bdba3b10fb73792d5fe))
* **ui:** posts made before the main window wait for it ([#535](https://github.com/coredev-uk/sona-go/issues/535)) ([15f8614](https://github.com/coredev-uk/sona-go/commit/15f8614e6fc1b5b335edfca6a710d7d33bb295af))
* **ui:** recent searches play a page-less station and list the same entries in the field and the shelf ([#514](https://github.com/coredev-uk/sona-go/issues/514)) ([ec928e9](https://github.com/coredev-uk/sona-go/commit/ec928e9e11ad02448a7dace93487db01674a7ecd))
* **ui:** the mini player is a remote: its menu covers every line of text ([#498](https://github.com/coredev-uk/sona-go/issues/498)) ([662d5a1](https://github.com/coredev-uk/sona-go/commit/662d5a19b2a322defd8f86084eb070c7f5aa819c))
* **updates:** the updated version starts after the old one quits ([#534](https://github.com/coredev-uk/sona-go/issues/534)) ([5803897](https://github.com/coredev-uk/sona-go/commit/5803897a5c43386fb14c83d0cc17dc845b52d6db))
* **windows:** --remove-sandbox also removes the add-ons' AppContainer profiles ([#554](https://github.com/coredev-uk/sona-go/issues/554)) ([f98bc74](https://github.com/coredev-uk/sona-go/commit/f98bc74de3071a4f299a144d2e00cbfb055c1af9))


### Performance

* **artwork:** return the heap's idle pages once a burst of artwork work ends ([#507](https://github.com/coredev-uk/sona-go/issues/507)) ([8c12da0](https://github.com/coredev-uk/sona-go/commit/8c12da0ffb54f5851d97b57b52a43f0b08160a3a))
* **discover:** the progress repaints when its fill moves a pixel ([#538](https://github.com/coredev-uk/sona-go/issues/538)) ([3d36d9c](https://github.com/coredev-uk/sona-go/commit/3d36d9ce1eecc23d786d0c1897ba5dc297f3d748))

## [0.6.0](https://github.com/coredev-uk/sona-go/compare/v0.5.0...v0.6.0) (2026-11-16)


### Features

* **addons:** manifests, permissions and the supervisor ([#492](https://github.com/coredev-uk/sona-go/issues/492)) ([79923e8](https://github.com/coredev-uk/sona-go/commit/79923e80d20b5fbeec77fd3f1abdce9f0231179a))
* **addons:** protocol v2 and docs/addons.md ([#488](https://github.com/coredev-uk/sona-go/issues/488)) ([5903d44](https://github.com/coredev-uk/sona-go/commit/5903d44170ea1d347d85ee2cdf62f5f76332c986))
* an opt-in artist tracker for new releases from favourite artists ([#456](https://github.com/coredev-uk/sona-go/issues/456)) ([45fe2e4](https://github.com/coredev-uk/sona-go/commit/45fe2e4c1461e1adc152f77253c5a478cc7b776d))
* **discover:** the lead stands beside a queue of the next cards, with its progress ([#490](https://github.com/coredev-uk/sona-go/issues/490)) ([45811a3](https://github.com/coredev-uk/sona-go/commit/45811a3297f3e60bf27ddef01281b835b712e197))
* **library:** pick up playlist changes from other devices ([#454](https://github.com/coredev-uk/sona-go/issues/454)) ([8f0100c](https://github.com/coredev-uk/sona-go/commit/8f0100c13a675cfd47346aa3176d4163ab2222dc))
* **playlist:** suggested songs leave when added, refresh, and start collapsed ([#458](https://github.com/coredev-uk/sona-go/issues/458)) ([5aa2fb9](https://github.com/coredev-uk/sona-go/commit/5aa2fb92edd6e4b728e6698851561fb1ffe4da09))
* **sidebar:** list collaborative playlists under Playlists ([#463](https://github.com/coredev-uk/sona-go/issues/463)) ([adea19a](https://github.com/coredev-uk/sona-go/commit/adea19ad565eae103d78e0e569b54b79a128b7fe))
* **ui:** a page-less station is one control, one cue, and pauses while it plays ([#522](https://github.com/coredev-uk/sona-go/issues/522)) ([d2b50a4](https://github.com/coredev-uk/sona-go/commit/d2b50a4bb905b31c8766f3c8dd863c9f10ed9cd9))
* **ui:** a web-only search result opens the browser ([#501](https://github.com/coredev-uk/sona-go/issues/501)) ([810a5dc](https://github.com/coredev-uk/sona-go/commit/810a5dce81f7496dbfb9567d48a6b8473ecc9c47))
* **ui:** every playable compact result row has a Play button ([#513](https://github.com/coredev-uk/sona-go/issues/513)) ([5f4316d](https://github.com/coredev-uk/sona-go/commit/5f4316d79c1cba0e588c78b0ab38363641991c6f))
* **ui:** inspect media items in development builds ([#465](https://github.com/coredev-uk/sona-go/issues/465)) ([1d28eac](https://github.com/coredev-uk/sona-go/commit/1d28eacfa92a6b6680a184570e147ae372cf7324))
* **ui:** kit.TrackList gets a rank column and an extra column slot, used by Discover charts ([#380](https://github.com/coredev-uk/sona-go/issues/380)) ([72216e0](https://github.com/coredev-uk/sona-go/commit/72216e054619c04df519d5e73642f723ae12a733))
* **ui:** mark uploaded songs among catalogue songs, and never show Disc 0 ([#398](https://github.com/coredev-uk/sona-go/issues/398)) ([0482363](https://github.com/coredev-uk/sona-go/commit/0482363d08cab77c977b3e3daaf3feaf465e3af6))
* **ui:** merge Made for you and Apple Music into one sidebar section ([#394](https://github.com/coredev-uk/sona-go/issues/394)) ([d0df2a0](https://github.com/coredev-uk/sona-go/commit/d0df2a0c62fef75bc8186ba701b42744363db8d6))
* **ui:** one 'opens in your browser' cue on tiles, cards and rows ([#494](https://github.com/coredev-uk/sona-go/issues/494)) ([697fcef](https://github.com/coredev-uk/sona-go/commit/697fcef53f3c6f5dddd45f80816efbc4747ab2c2))
* **ui:** palette opens a library song's page and cues what Enter does ([#493](https://github.com/coredev-uk/sona-go/issues/493)) ([d4e10e9](https://github.com/coredev-uk/sona-go/commit/d4e10e92feec738ad21eba532e8d0406618a37fe))
* **ui:** selectable text where people copy it, with an ISRC copy button ([#451](https://github.com/coredev-uk/sona-go/issues/451)) ([bcf3536](https://github.com/coredev-uk/sona-go/commit/bcf353611e6875145eb25c9be62fa3f90325b694))
* **ui:** selectable text where people copy it, with an ISRC copy button ([#451](https://github.com/coredev-uk/sona-go/issues/451)) ([8325c7f](https://github.com/coredev-uk/sona-go/commit/8325c7f549080266dd92ab0f54f0bce2e678b2cf))
* **ui:** station rows pause from the equaliser and have a menu ([#519](https://github.com/coredev-uk/sona-go/issues/519)) ([2ca8eb8](https://github.com/coredev-uk/sona-go/commit/2ca8eb8f837a02f4f2c7e93e0214e891cbf78ffb))
* **ui:** suggestion rows cue play and leave-Sona, not just open ([#495](https://github.com/coredev-uk/sona-go/issues/495)) ([45f138b](https://github.com/coredev-uk/sona-go/commit/45f138b34206f6144d24b9b62d6b86700fcd8426))
* **ui:** the playlist cover picker says up front that covers stay in Sona ([#381](https://github.com/coredev-uk/sona-go/issues/381)) ([5e02b60](https://github.com/coredev-uk/sona-go/commit/5e02b607208f6c2997bf5d22f957d67d0a3102bf))
* **ui:** the queue's Playing from link is a Tab stop and rows link their title and artist ([#491](https://github.com/coredev-uk/sona-go/issues/491)) ([a0dfc2d](https://github.com/coredev-uk/sona-go/commit/a0dfc2dfb78ee6f8b3ce7cda56abd06bd95d3d99))
* **ui:** titles in Title Case, in Apple's style ([#469](https://github.com/coredev-uk/sona-go/issues/469)) ([0f22b3a](https://github.com/coredev-uk/sona-go/commit/0f22b3acc4792f06c3a4060eb92922d98785d600))
* **windows:** Discord status over the named pipe ([#412](https://github.com/coredev-uk/sona-go/issues/412)) ([9dc8df9](https://github.com/coredev-uk/sona-go/commit/9dc8df9a0c8f3e9882909d8998b9453fe0d19f24))
* **windows:** open Apple Music links in Sona ([#471](https://github.com/coredev-uk/sona-go/issues/471)) ([fab99bb](https://github.com/coredev-uk/sona-go/commit/fab99bb446b7cf38c619d876175bae674f2bbd78))
* **windows:** settings export and import through the file dialogs ([#470](https://github.com/coredev-uk/sona-go/issues/470)) ([9007e41](https://github.com/coredev-uk/sona-go/commit/9007e41dd0b0d7d644443a77c8da7577d63bc5de))
* **windows:** start Sona when you log in ([#403](https://github.com/coredev-uk/sona-go/issues/403)) ([99cd43d](https://github.com/coredev-uk/sona-go/commit/99cd43d31b58fcaa08577928cac824235a386d8f))
* **windows:** stay awake while playing and the on-battery state ([#455](https://github.com/coredev-uk/sona-go/issues/455)) ([e5ec46e](https://github.com/coredev-uk/sona-go/commit/e5ec46e272e925dc0dc59979269eebf73961f676))
* **windows:** track notifications ([#401](https://github.com/coredev-uk/sona-go/issues/401)) ([88493b4](https://github.com/coredev-uk/sona-go/commit/88493b421761d0353021f03e3ab9464f2ed4b062))


### Bug Fixes

* **a11y:** drop announcements that repeat a status MyGo reads ([#393](https://github.com/coredev-uk/sona-go/issues/393)) ([40901f2](https://github.com/coredev-uk/sona-go/commit/40901f2e5cf6a9039fd7dcc61bea708cb0c09433))
* **addons:** hide error text, judge answers again, drop hidden queue slots ([#517](https://github.com/coredev-uk/sona-go/issues/517)) ([e3ad6f0](https://github.com/coredev-uk/sona-go/commit/e3ad6f039bc81ec14c46bfea5a150fb3b206d44c))
* **addons:** judge each add-on event on a fresh snapshot, not an old one ([#525](https://github.com/coredev-uk/sona-go/issues/525)) ([fc4bd27](https://github.com/coredev-uk/sona-go/commit/fc4bd2758394f048cabd25239e3210a7dccb598f))
* **discover:** the hero shows Apple's line for each card ([#486](https://github.com/coredev-uk/sona-go/issues/486)) ([01894ee](https://github.com/coredev-uk/sona-go/commit/01894ee90b3efafc3d782365e08f0e148cb35dee))
* **home:** Top Picks show Apple's editorial name and line for stations and episodes ([#477](https://github.com/coredev-uk/sona-go/issues/477)) ([62e87cd](https://github.com/coredev-uk/sona-go/commit/62e87cdf189019125505b5d20913aeeb38d16f6c))
* **library:** a personal mix takes its freshness from its catalogue playlist ([#414](https://github.com/coredev-uk/sona-go/issues/414)) ([cba9a4d](https://github.com/coredev-uk/sona-go/commit/cba9a4de4515e1633bdd14bca0aae2bab2f81cb8))
* **library:** deleting a library song removes it at once and drops its emptied album ([#468](https://github.com/coredev-uk/sona-go/issues/468)) ([8c53115](https://github.com/coredev-uk/sona-go/commit/8c531153c687cc19a2bdff317f66f46493c66bc5))
* **library:** every library album offers Delete from library ([#457](https://github.com/coredev-uk/sona-go/issues/457)) ([357031b](https://github.com/coredev-uk/sona-go/commit/357031b68d523cfe49d5c83f7431e47eee9bbe8f))
* **library:** leave unreleased placeholder songs out of every song list ([#428](https://github.com/coredev-uk/sona-go/issues/428)) ([0e64b7b](https://github.com/coredev-uk/sona-go/commit/0e64b7b03f56c6585f679de4b5e910f394652df2))
* **library:** show an offline state on Music videos instead of a fixed note ([#400](https://github.com/coredev-uk/sona-go/issues/400)) ([5962905](https://github.com/coredev-uk/sona-go/commit/59629059dc2b653760b6c086586c330a3c58e191))
* **radio:** the narrow station skeleton builds one loading root ([#452](https://github.com/coredev-uk/sona-go/issues/452)) ([294c837](https://github.com/coredev-uk/sona-go/commit/294c837b7924487531a2fa9c1edf8989a4869a0d))
* **services:** keep the case of Apple's names; sentence-case only generic badge words ([#460](https://github.com/coredev-uk/sona-go/issues/460)) ([d500cc4](https://github.com/coredev-uk/sona-go/commit/d500cc4071dbd9cb1e8ea614d18672585711cf5d))
* **services:** show a library playlist's own cover in Recently Played ([#395](https://github.com/coredev-uk/sona-go/issues/395)) ([863d92e](https://github.com/coredev-uk/sona-go/commit/863d92e195e7485e5e2a7dbedd46b336c35fc64a))
* **services:** show a recently searched library playlist's own cover ([#422](https://github.com/coredev-uk/sona-go/issues/422)) ([d5d590e](https://github.com/coredev-uk/sona-go/commit/d5d590e190979fd8e3b05cfeef346727690e2d93))
* **services:** show a wide station's cover as its centre square ([#431](https://github.com/coredev-uk/sona-go/issues/431)) ([4a9cf1c](https://github.com/coredev-uk/sona-go/commit/4a9cf1c47ef94d258a7d09f6b78f71192b255cd4))
* **ui:** a double-click runs an item's action once ([#515](https://github.com/coredev-uk/sona-go/issues/515)) ([fb89439](https://github.com/coredev-uk/sona-go/commit/fb89439a880bb2de17ce2ec9c1172256a40c9db6))
* **ui:** a sidebar pin offers only what it does ([#487](https://github.com/coredev-uk/sona-go/issues/487)) ([8dd78df](https://github.com/coredev-uk/sona-go/commit/8dd78df78f0815526274ba5e91e73a2ac5c50485))
* **ui:** a tile's overlays are placed from the cover's whole box ([#506](https://github.com/coredev-uk/sona-go/issues/506)) ([e687009](https://github.com/coredev-uk/sona-go/commit/e687009561c48c092ddaf6adef4d5cdbde71f691))
* **ui:** a track row's play wash and a wide card's play disc fade, and the wash shows on focus ([#504](https://github.com/coredev-uk/sona-go/issues/504)) ([792101c](https://github.com/coredev-uk/sona-go/commit/792101c2fa16afd1f1c3fef7167dee2c32b3b283))
* **ui:** crop Discover bricks to their frame ([#450](https://github.com/coredev-uk/sona-go/issues/450)) ([c08550a](https://github.com/coredev-uk/sona-go/commit/c08550aa89514eebdb6ab1526d4db68c7efa5cbb))
* **ui:** every play mark draws the solid triangle ([#496](https://github.com/coredev-uk/sona-go/issues/496)) ([bf73d25](https://github.com/coredev-uk/sona-go/commit/bf73d256275833ae5943d4b5be24a2f95184db88))
* **ui:** kit.Artwork rounds its boxes to whole pixels ([#384](https://github.com/coredev-uk/sona-go/issues/384)) ([0eba8b2](https://github.com/coredev-uk/sona-go/commit/0eba8b2f932b3a708517deb9b0df0cb716a3ce5e))
* **ui:** Music videos' Try again re-checks the connection and offline shows after a failed reload ([#499](https://github.com/coredev-uk/sona-go/issues/499)) ([587d872](https://github.com/coredev-uk/sona-go/commit/587d872ae098966c2b0e0a9a4a8170c6fdbda8f1))
* **ui:** palette Enter runs the first entry the list shows ([#483](https://github.com/coredev-uk/sona-go/issues/483)) ([66c86b4](https://github.com/coredev-uk/sona-go/commit/66c86b47b6ea4704d9536446feae337417c4092f))
* **ui:** Play on a tile, link tile, station row and artist link is remembered as recently searched ([#508](https://github.com/coredev-uk/sona-go/issues/508)) ([e396243](https://github.com/coredev-uk/sona-go/commit/e396243ad2ef0b61fd654237426fae0dfc17959a))
* **ui:** playlist header says Added, not Created, for playlists the user didn't make ([#397](https://github.com/coredev-uk/sona-go/issues/397)) ([c01278c](https://github.com/coredev-uk/sona-go/commit/c01278c78884e4059a1007f20db3f4e1ce43cd59))
* **ui:** put the equaliser bars on the device pixel grid ([#484](https://github.com/coredev-uk/sona-go/issues/484)) ([17a692a](https://github.com/coredev-uk/sona-go/commit/17a692a748394f00ff5cc077ba509ffd9b023f51))
* **ui:** right-click opens the item menu wherever a ⋯ button does ([#392](https://github.com/coredev-uk/sona-go/issues/392)) ([89c8df2](https://github.com/coredev-uk/sona-go/commit/89c8df28e5a7cb87074af52be0a614017b33991e))
* **ui:** shelf track rows select on click like track lists ([#485](https://github.com/coredev-uk/sona-go/issues/485)) ([d54f431](https://github.com/coredev-uk/sona-go/commit/d54f431a6841dfa11a328c68afd27c6c062335d4))


### Performance

* **artwork:** fetch sources at the sizes Apple's CDN already holds ([#420](https://github.com/coredev-uk/sona-go/issues/420)) ([859bedf](https://github.com/coredev-uk/sona-go/commit/859bedf45b25f006b278e83ecd8a86effeb640ea))

## [0.5.0](https://github.com/coredev-uk/sona-go/compare/v0.4.0...v0.5.0) (2026-11-15)


### Features

* **album:** a band in the cover's colour replaces the sleeve column ([#287](https://github.com/coredev-uk/sona-go/issues/287)) ([dbb2c9d](https://github.com/coredev-uk/sona-go/commit/dbb2c9d1dda111a22dcdb34c600d3ccfee1e1f26))
* **media:** motion artwork through the OS's own video engine on macOS and Windows ([#264](https://github.com/coredev-uk/sona-go/issues/264)) ([386db0c](https://github.com/coredev-uk/sona-go/commit/386db0cfd1be6cfc49b0d74138f62a896c768582))
* **player:** scroll over the volume to change it ([#329](https://github.com/coredev-uk/sona-go/issues/329)) ([f606b0d](https://github.com/coredev-uk/sona-go/commit/f606b0dee3eef10ab627c5c5321dbb6a84452b95))
* **player:** the album in the player bar opens its page ([#346](https://github.com/coredev-uk/sona-go/issues/346)) ([cdbeec1](https://github.com/coredev-uk/sona-go/commit/cdbeec144bb9d86fff34ec784b93f3729633eb78))
* **playlist:** show when a playlist was created, beside updated ([#352](https://github.com/coredev-uk/sona-go/issues/352)) ([821f28d](https://github.com/coredev-uk/sona-go/commit/821f28dbe1997443b9440c486e1cb17d1dfc4f92))
* **theme:** a neutral board and a play colour at 3:1 in light themes ([#282](https://github.com/coredev-uk/sona-go/issues/282)) ([1e0e09d](https://github.com/coredev-uk/sona-go/commit/1e0e09d3598073d8f7ba77524a0a001b04147602))
* **theme:** an accent selector for Sona's own themes, OLED in blue ([#286](https://github.com/coredev-uk/sona-go/issues/286)) ([378a5e5](https://github.com/coredev-uk/sona-go/commit/378a5e5e751ce7ad997efc46da89a65859a0785c))
* **ui:** choose which artist to open when a song or album has several ([#353](https://github.com/coredev-uk/sona-go/issues/353)) ([7ec77c2](https://github.com/coredev-uk/sona-go/commit/7ec77c2e2c30de041ac7fb96b63bee19eaba19ed))
* **ui:** redesign every page, add themes and sidebar shuffle ([#277](https://github.com/coredev-uk/sona-go/issues/277)) ([6710eea](https://github.com/coredev-uk/sona-go/commit/6710eea90bef1689932ea034a335e4429db5d581))
* use MyGo v0.9.0's session-end quit, KeepRunning, DefaultApp and GPU YUV ([#263](https://github.com/coredev-uk/sona-go/issues/263)) ([bd792ad](https://github.com/coredev-uk/sona-go/commit/bd792ad3bbfc507e1d13d6561ad3bb7f8b8fdd31))


### Bug Fixes

* **album:** a released song on an upcoming album plays, by its own playParams ([#292](https://github.com/coredev-uk/sona-go/issues/292)) ([ab2a7bb](https://github.com/coredev-uk/sona-go/commit/ab2a7bb87fd93274a55ca8453e4268a22409c6f3))
* **ci:** pin Proton's secrets action to the commit that ships its build ([#290](https://github.com/coredev-uk/sona-go/issues/290)) ([f52508f](https://github.com/coredev-uk/sona-go/commit/f52508f6da57206772f250be07e5bc0048cf8136))
* **deps:** update module github.com/pressly/goose/v3 to v3.30.1 ([#275](https://github.com/coredev-uk/sona-go/issues/275)) ([ce10f8a](https://github.com/coredev-uk/sona-go/commit/ce10f8aec506de051c7358031c4644677a1e2b0e))
* **deps:** update module golang.org/x/mod to v0.43.0 ([#274](https://github.com/coredev-uk/sona-go/issues/274)) ([dfe6336](https://github.com/coredev-uk/sona-go/commit/dfe633613a27f11236d4b97ad0f6f0d1636e578e))
* **deps:** update MyGo to v0.9.0 ([#262](https://github.com/coredev-uk/sona-go/issues/262)) ([0a33597](https://github.com/coredev-uk/sona-go/commit/0a3359718ac7da259566795e2be11195167923bd))
* **discover:** a click in the lead no longer stops it advancing ([#281](https://github.com/coredev-uk/sona-go/issues/281)) ([2730944](https://github.com/coredev-uk/sona-go/commit/2730944f8613c893917d99a5a5705c70eed3681d))
* **i18n:** reject JS \u{} escapes past U+10FFFF ([#364](https://github.com/coredev-uk/sona-go/issues/364)) ([151fd3e](https://github.com/coredev-uk/sona-go/commit/151fd3e22e27fb130aa845c8e1428e52ea83f2b8))
* **playback:** a skip supersedes a slow load instead of queueing behind it ([#260](https://github.com/coredev-uk/sona-go/issues/260)) ([289065b](https://github.com/coredev-uk/sona-go/commit/289065b6bf5accdf32a6925410dfc68d31e42260))
* **player:** the volume wheel moves 2% a notch, on every kind of wheel ([#351](https://github.com/coredev-uk/sona-go/issues/351)) ([386ab00](https://github.com/coredev-uk/sona-go/commit/386ab004f73c0cd8b0bd5d7d88d564874addfc93))
* **search:** choosing a song opens its page; Play plays it ([#332](https://github.com/coredev-uk/sona-go/issues/332)) ([dec31e2](https://github.com/coredev-uk/sona-go/commit/dec31e27291b337e8a99be9d8ee4b5f7bdfcb442))
* **ui:** centre the play triangle by one rule, sidebar and Play discs ([#331](https://github.com/coredev-uk/sona-go/issues/331)) ([f5b5682](https://github.com/coredev-uk/sona-go/commit/f5b5682a57f22d73a3635731bd4930deefaefc7d))
* **ui:** no eyebrow in Search's best match, no rank numbers on Home ([#280](https://github.com/coredev-uk/sona-go/issues/280)) ([12767c9](https://github.com/coredev-uk/sona-go/commit/12767c938960de64e94f8ec7f2b5466a040d2a3e))
* **update:** Windows updates install: no './' in the archive, the host let go, the sandbox re-granted ([#285](https://github.com/coredev-uk/sona-go/issues/285)) ([6796209](https://github.com/coredev-uk/sona-go/commit/6796209626782e1a682f1f65d4b48f48e7891456))


### Performance

* **playback:** fetch a queue's unstored catalogue songs in one batched request ([#284](https://github.com/coredev-uk/sona-go/issues/284)) ([1d29a60](https://github.com/coredev-uk/sona-go/commit/1d29a607ec82184c0b2e5f837bc397c7bcce6db5))
* **ui:** the playing glyph moves in sub-pixel steps at the display's rate ([#296](https://github.com/coredev-uk/sona-go/issues/296)) ([6fb55ab](https://github.com/coredev-uk/sona-go/commit/6fb55ab1331f442c9b179b0992fd0b3f26d6b80f))
* **ui:** the playing glyph moves smoothly instead of stepping three times a second ([#283](https://github.com/coredev-uk/sona-go/issues/283)) ([bed4160](https://github.com/coredev-uk/sona-go/commit/bed4160c60e7782e4c5712600d753e04737c8ccb))

## [0.4.0](https://github.com/coredev-uk/sona-go/compare/v0.3.2...v0.4.0) (2026-11-11)


### Features

* **album:** redesign the not-out-yet album page ([#251](https://github.com/coredev-uk/sona-go/issues/251)) ([529f3b0](https://github.com/coredev-uk/sona-go/commit/529f3b0b8a293feb853e3aae8548fad7b6a20f5c))
* **cli:** sona lyrics prints and follows the playing song's lyrics ([#253](https://github.com/coredev-uk/sona-go/issues/253)) ([a9979ec](https://github.com/coredev-uk/sona-go/commit/a9979ec0e9bbea677e5ae729238a697df8070684))
* **media:** motion artwork on Windows ([#231](https://github.com/coredev-uk/sona-go/issues/231)) ([0c64349](https://github.com/coredev-uk/sona-go/commit/0c643491d1a430547dc05b4a5beb799ee8b305f8))
* **playback:** the CDM host and playspike on Windows (licence prototype) ([#220](https://github.com/coredev-uk/sona-go/issues/220)) ([b77076d](https://github.com/coredev-uk/sona-go/commit/b77076d5b76c6ad92211cd05e2ffb4f898e317d2))
* **playback:** Windows playback: engine, AppContainer sandbox, packaging ([#222](https://github.com/coredev-uk/sona-go/issues/222)) ([3305485](https://github.com/coredev-uk/sona-go/commit/33054850b601e6cf895353ced754b07e484327f9))
* **playlist:** readable Updated dates in calendar words ([#244](https://github.com/coredev-uk/sona-go/issues/244)) ([7017a21](https://github.com/coredev-uk/sona-go/commit/7017a21176b9fd59801c37770602c34de96beb84))
* **playlist:** sort and find the songs on the playlist page ([#250](https://github.com/coredev-uk/sona-go/issues/250)) ([cb8a73f](https://github.com/coredev-uk/sona-go/commit/cb8a73f5c0d6ff18b2c4737fccb72b730fabe478))
* **theme:** SF Symbols for the controls on macOS ([#246](https://github.com/coredev-uk/sona-go/issues/246)) ([9baf4c3](https://github.com/coredev-uk/sona-go/commit/9baf4c30b77ee7f7033007ebe36de3ef6d5adc00))
* **ui:** ? opens the shortcut help on GTK and AppKit ([#245](https://github.com/coredev-uk/sona-go/issues/245)) ([5063e69](https://github.com/coredev-uk/sona-go/commit/5063e6981fbff7136c40b921e3025515eda05127))


### Bug Fixes

* **deps:** update golang.org/x/sys to v0.50.0 and golang.org/x/term to v0.48.0 ([#243](https://github.com/coredev-uk/sona-go/issues/243)) ([655dd22](https://github.com/coredev-uk/sona-go/commit/655dd22ccd9a6b356be4d0a2328cb64083ceb8fe))
* **deps:** update module github.com/pressly/goose/v3 to v3.30.0 ([#241](https://github.com/coredev-uk/sona-go/issues/241)) ([a6bdb6a](https://github.com/coredev-uk/sona-go/commit/a6bdb6a9227583984773d03ba80aada7f3daf528))
* **deps:** update module golang.org/x/text to v0.44.0 ([#238](https://github.com/coredev-uk/sona-go/issues/238)) ([f5d7840](https://github.com/coredev-uk/sona-go/commit/f5d78400642d9d61ec89148fda2d2215643d7bdb))
* **library:** a deleted playlist leaves Recently added ([#242](https://github.com/coredev-uk/sona-go/issues/242)) ([8e3a646](https://github.com/coredev-uk/sona-go/commit/8e3a646d09fc9e663e871002764eb7ca4098671e))
* **library:** a pre-added album shows as not out, with its unreleased songs unplayable ([#252](https://github.com/coredev-uk/sona-go/issues/252)) ([e85649a](https://github.com/coredev-uk/sona-go/commit/e85649ae516e1a672c727b7bb815d05cba82520f))
* **nix:** install the release build from the flake ([#247](https://github.com/coredev-uk/sona-go/issues/247)) ([8bc6e30](https://github.com/coredev-uk/sona-go/commit/8bc6e30e2d39cd84a23e40a582cc9fe6c783c801))
* **nix:** update vendorHash for the go-sqlite3 and MyGo bumps ([#239](https://github.com/coredev-uk/sona-go/issues/239)) ([7f798b9](https://github.com/coredev-uk/sona-go/commit/7f798b9d37e1d05be126a9d920e5642dcf150df8))
* **windows:** uninstall removes the playback sandbox; retry preview playback on network errors ([#230](https://github.com/coredev-uk/sona-go/issues/230)) ([050df46](https://github.com/coredev-uk/sona-go/commit/050df4658b3c50843c33aa708090fd40f1fff26e))

## [0.3.2](https://github.com/coredev-uk/sona-go/compare/v0.3.1...v0.3.2) (2026-11-08)


### Bug Fixes

* **deps:** update module github.com/mattn/go-sqlite3 to v1.14.53 ([#175](https://github.com/coredev-uk/sona-go/issues/175)) ([992c5fb](https://github.com/coredev-uk/sona-go/commit/992c5fb10adff5de12bf3251b1a7f0f975a8c7e5))

## [0.3.1](https://github.com/coredev-uk/sona-go/compare/v0.3.0...v0.3.1) (2026-11-08)


### Bug Fixes

* **release:** the release build gets the release environment's secrets ([#214](https://github.com/coredev-uk/sona-go/issues/214)) ([6401af7](https://github.com/coredev-uk/sona-go/commit/6401af77a5e082ade1a7b4947ebdebe92677027a))

## [0.3.0](https://github.com/coredev-uk/sona-go/compare/v0.2.0...v0.3.0) (2026-11-08)


### ⚠ BREAKING CHANGES

* MyGo native UI replaces Wails and React ([#93](https://github.com/coredev-uk/sona-go/issues/93))

### Features

* **a11y:** focus targets that are not Tab stops ([#184](https://github.com/coredev-uk/sona-go/issues/184)) ([4c47280](https://github.com/coredev-uk/sona-go/commit/4c47280c85a5d758248e13b99105f6d6571aad5b))
* **a11y:** list box and option roles in search, artists filter and palette popups ([#205](https://github.com/coredev-uk/sona-go/issues/205)) ([950911d](https://github.com/coredev-uk/sona-go/commit/950911d3631f701c04837f668223e9e2d6016157))
* **a11y:** music video title is a heading holding its link ([#204](https://github.com/coredev-uk/sona-go/issues/204)) ([fd7e922](https://github.com/coredev-uk/sona-go/commit/fd7e922315a563180b961e922a48fec8caef4f47))
* **a11y:** say each cover tile, queue row and sidebar item's place in its set ([#206](https://github.com/coredev-uk/sona-go/issues/206)) ([9c53d73](https://github.com/coredev-uk/sona-go/commit/9c53d73d63faf441db3188fd4ae6f66315988199))
* **a11y:** search and region landmarks for the search field, filter bars and player surfaces ([#209](https://github.com/coredev-uk/sona-go/issues/209)) ([c67e601](https://github.com/coredev-uk/sona-go/commit/c67e6019348ed37cbc8836333f738f1e355d0ee8))
* **a11y:** track list sort direction and grid semantics ([#207](https://github.com/coredev-uk/sona-go/issues/207)) ([dbc54b6](https://github.com/coredev-uk/sona-go/commit/dbc54b6e896b40ca07c4a8fd870dcd97dfd6b30f))
* **a11y:** use MyGo v0.8.1's accessibility states ([#187](https://github.com/coredev-uk/sona-go/issues/187)) ([41ea08e](https://github.com/coredev-uk/sona-go/commit/41ea08e4a6692741b8a86fd00e77ae9d7180fb5c))
* **account:** native private listening, Widevine, updates, reminders and motion ([#101](https://github.com/coredev-uk/sona-go/issues/101)) ([a1ecbab](https://github.com/coredev-uk/sona-go/commit/a1ecbabd74351586c22ad2637e23eb4a1aeb4c00))
* **auth:** mention Touch ID passkeys on the macOS browser sign-in ([#162](https://github.com/coredev-uk/sona-go/issues/162)) ([3069372](https://github.com/coredev-uk/sona-go/commit/3069372aa517b75407825e52a5d64b14f7a69937))
* **auth:** native sign-in, account and first-run setup ([#103](https://github.com/coredev-uk/sona-go/issues/103)) ([188bd3c](https://github.com/coredev-uk/sona-go/commit/188bd3c67ab2b59a8acc1a4e4f8b21a59323ccc0))
* **browse:** native home, discover and radio ([#112](https://github.com/coredev-uk/sona-go/issues/112)) ([b37c32f](https://github.com/coredev-uk/sona-go/commit/b37c32f9af5c9746a40c762ace3aa1eb6e757eeb))
* **catalog:** native album, artist and song pages ([#107](https://github.com/coredev-uk/sona-go/issues/107)) ([cde732c](https://github.com/coredev-uk/sona-go/commit/cde732c8408b7d4f0801bf96b2aec58b9a62db42))
* **config:** settings that lived in the web view ([#119](https://github.com/coredev-uk/sona-go/issues/119)) ([743c820](https://github.com/coredev-uk/sona-go/commit/743c820cac8812687fce652391f2b4ffa0d9deb3))
* **dev:** dev loop, gallery and headless harness ([#99](https://github.com/coredev-uk/sona-go/issues/99)) ([6165ff3](https://github.com/coredev-uk/sona-go/commit/6165ff3a0ffa50d9991748de3aea8662957d82df))
* **i18n:** native catalogue checks ([#118](https://github.com/coredev-uk/sona-go/issues/118)) ([43f6b56](https://github.com/coredev-uk/sona-go/commit/43f6b560dab39a67b0610efbe2fbea6e67ff30bd))
* **i18n:** shared duration and date formatters ([#114](https://github.com/coredev-uk/sona-go/issues/114)) ([7344c76](https://github.com/coredev-uk/sona-go/commit/7344c76f30c1b2ee37d742543f80f34a1730b292))
* **kit:** busy states and behaviour slips ([#125](https://github.com/coredev-uk/sona-go/issues/125)) ([afef73f](https://github.com/coredev-uk/sona-go/commit/afef73f14cf31091a0fa5b0526b0f8c019d19825))
* **library:** native library ([#110](https://github.com/coredev-uk/sona-go/issues/110)) ([15b8798](https://github.com/coredev-uk/sona-go/commit/15b8798db8a04d994c09b5a8ef718128b42a6313))
* **lyrics:** shape word-timed lyrics in their language ([#193](https://github.com/coredev-uk/sona-go/issues/193)) ([c993e0a](https://github.com/coredev-uk/sona-go/commit/c993e0ab1cb4f94cdf501551b34f481168d47cf7))
* **macos:** let the user make Sona the default for Apple Music links ([#198](https://github.com/coredev-uk/sona-go/issues/198)) ([b9ec0aa](https://github.com/coredev-uk/sona-go/commit/b9ec0aa36b204bd959328bb5f43d3845cb7d80ad))
* **macos:** notifications, sleep inhibit, login item, file dialogs and power source ([#142](https://github.com/coredev-uk/sona-go/issues/142)) ([48c0236](https://github.com/coredev-uk/sona-go/commit/48c02366f86cc936d34f296e7459ab27ae9a8393))
* **media:** media keys and Now Playing on macOS and Windows ([#122](https://github.com/coredev-uk/sona-go/issues/122)) ([98a7323](https://github.com/coredev-uk/sona-go/commit/98a7323279d70d7a7adbf4b2eaceb6ffb14c4f25))
* **playback:** choose the output device on macOS and follow the default output ([#188](https://github.com/coredev-uk/sona-go/issues/188)) ([10673c0](https://github.com/coredev-uk/sona-go/commit/10673c0592e23bf28ccce853741d92ebb34fa675))
* **player:** context menu on the playing item ([#153](https://github.com/coredev-uk/sona-go/issues/153)) ([a647ee7](https://github.com/coredev-uk/sona-go/commit/a647ee72e6d64884faf6b747f0477f06696ab22f))
* **player:** native Now Playing and lyrics ([#105](https://github.com/coredev-uk/sona-go/issues/105)) ([013155a](https://github.com/coredev-uk/sona-go/commit/013155ad1aa67a0b24494d6c13cb1bfc1fd67077))
* **player:** native player bar and queue ([#113](https://github.com/coredev-uk/sona-go/issues/113)) ([3d7d277](https://github.com/coredev-uk/sona-go/commit/3d7d2774720640b5a8a1458e1a64f52c296a31b0))
* **playlist:** choose a library playlist's cover ([#158](https://github.com/coredev-uk/sona-go/issues/158)) ([25a60d9](https://github.com/coredev-uk/sona-go/commit/25a60d9d49aa7c0895c667467ad91fd6e2634949))
* **playlist:** native playlists ([#108](https://github.com/coredev-uk/sona-go/issues/108)) ([412eee1](https://github.com/coredev-uk/sona-go/commit/412eee1d84e68c638fb5d1594dd979ff30bb3a4a))
* **search:** native search and concerts ([#109](https://github.com/coredev-uk/sona-go/issues/109)) ([0777a30](https://github.com/coredev-uk/sona-go/commit/0777a30063b649c2f303d92c7df2b7b3418fe6a7))
* **settings:** native settings ([#104](https://github.com/coredev-uk/sona-go/issues/104)) ([0e593a3](https://github.com/coredev-uk/sona-go/commit/0e593a327e39ca3e44871aacfecd7decb4185bf1))
* **shell:** native windows and desktop integration ([#116](https://github.com/coredev-uk/sona-go/issues/116)) ([d05abfb](https://github.com/coredev-uk/sona-go/commit/d05abfb4909e4b9d4cabc6b569bc21884ff4cdc5))
* **shell:** swipe between pages with two fingers on macOS ([#156](https://github.com/coredev-uk/sona-go/issues/156)) ([0ec915b](https://github.com/coredev-uk/sona-go/commit/0ec915bf5911de9920ce0bd580eecdfd283fe50b))
* **shell:** the macOS menu bar ([#140](https://github.com/coredev-uk/sona-go/issues/140)) ([ac1fe2f](https://github.com/coredev-uk/sona-go/commit/ac1fe2fbd8920ae636766b96345f5bb58a21572e))
* **shell:** the macOS menu bar's Go menu and keys, title-case tray items on macOS ([#151](https://github.com/coredev-uk/sona-go/issues/151)) ([4a51f91](https://github.com/coredev-uk/sona-go/commit/4a51f9169d860d187694c8fcb7d9c8c17bda3964))
* **theme:** cover palettes ([#121](https://github.com/coredev-uk/sona-go/issues/121)) ([942b305](https://github.com/coredev-uk/sona-go/commit/942b30576ada2e15c0f987c3c41ce7297722d101))
* **tools:** native equaliser, command palette and shortcut help ([#100](https://github.com/coredev-uk/sona-go/issues/100)) ([9506d1d](https://github.com/coredev-uk/sona-go/commit/9506d1d88753ef82f131c8d9a3910329bdb1e9f0))
* **ui:** kit collections ([#98](https://github.com/coredev-uk/sona-go/issues/98)) ([ec9b423](https://github.com/coredev-uk/sona-go/commit/ec9b4236ae98c44ab96a47b3edadb7c808aab9a2))
* **ui:** kit controls ([#96](https://github.com/coredev-uk/sona-go/issues/96)) ([a27beba](https://github.com/coredev-uk/sona-go/commit/a27beba6468beb031af77af774a5b89e775c367f))
* **ui:** kit media ([#97](https://github.com/coredev-uk/sona-go/issues/97)) ([91daa17](https://github.com/coredev-uk/sona-go/commit/91daa17ceb2ee31398a5030ad99531f0b4e3089e))
* **ui:** kit overlays ([#95](https://github.com/coredev-uk/sona-go/issues/95)) ([88fa8f0](https://github.com/coredev-uk/sona-go/commit/88fa8f0147b2d140bed109ed4554f7f7ef6c4e6a))
* **ui:** show tooltips after 400 ms ([#183](https://github.com/coredev-uk/sona-go/issues/183)) ([f050158](https://github.com/coredev-uk/sona-go/commit/f050158d0cb9ab16d1c8f87ecf6ded0eb775a4a6))
* **update:** restore the previous version from Settings → Diagnostics ([#212](https://github.com/coredev-uk/sona-go/issues/212)) ([64984f3](https://github.com/coredev-uk/sona-go/commit/64984f36839ab041c8063f4680ed54fbecfd43e4))
* **updates:** self-updates on macOS and Windows ([#123](https://github.com/coredev-uk/sona-go/issues/123)) ([5e24791](https://github.com/coredev-uk/sona-go/commit/5e24791b4edd553111065758155d9cc58fd69e3b))
* **video:** native music videos ([#111](https://github.com/coredev-uk/sona-go/issues/111)) ([d7f4528](https://github.com/coredev-uk/sona-go/commit/d7f4528f43a5e3cd550dec34b9ffa8ea5c76b4a1))


### Bug Fixes

* **a11y:** fixes from a real Orca walk of the app, and a dispatch-only Orca check ([#172](https://github.com/coredev-uk/sona-go/issues/172)) ([e3b51fc](https://github.com/coredev-uk/sona-go/commit/e3b51fc1faa23d2925d195588b13fde2ef41d718))
* **a11y:** native accessibility pass ([#117](https://github.com/coredev-uk/sona-go/issues/117)) ([f9885f7](https://github.com/coredev-uk/sona-go/commit/f9885f73bf85cea588498f508ce4c0a4f32cfc98))
* **a11y:** use what MyGo already allows ([#127](https://github.com/coredev-uk/sona-go/issues/127)) ([af89370](https://github.com/coredev-uk/sona-go/commit/af89370699435da42eba66be03338c8eabad1708))
* **ci:** main verified locally, CI restructured ([#137](https://github.com/coredev-uk/sona-go/issues/137)) ([63d0d40](https://github.com/coredev-uk/sona-go/commit/63d0d40f5d85509646ac83ebe6f7fa5d0e18ba2c))
* **config:** the failed-save test fails the save as root too ([#171](https://github.com/coredev-uk/sona-go/issues/171)) ([c97bd2e](https://github.com/coredev-uk/sona-go/commit/c97bd2e38f37c43751b08f1d7d5b734a18a7d880))
* **deps:** update module github.com/ebitengine/purego to v0.11.3 ([#94](https://github.com/coredev-uk/sona-go/issues/94)) ([7247039](https://github.com/coredev-uk/sona-go/commit/72470394604d63069880549c47bee91b1addf95f))
* **deps:** update MyGo to v0.8.1 ([#182](https://github.com/coredev-uk/sona-go/issues/182)) ([accdf5e](https://github.com/coredev-uk/sona-go/commit/accdf5efdfc2ecf6f6b344b3163d17a3b4f92029))
* **deps:** update MyGo to v0.8.3 ([#192](https://github.com/coredev-uk/sona-go/issues/192)) ([2d7fffa](https://github.com/coredev-uk/sona-go/commit/2d7fffabc004d958d11cc10553f28f895362f231))
* **devenv:** build Delve without its tests, which need ptrace ([#169](https://github.com/coredev-uk/sona-go/issues/169)) ([2d1ed1d](https://github.com/coredev-uk/sona-go/commit/2d1ed1dbf81a069cb63e592e38174ebbb050ce52))
* **discover:** hero thumbnails show their artwork whole, decoded at each window's own scale ([#161](https://github.com/coredev-uk/sona-go/issues/161)) ([9a97ac7](https://github.com/coredev-uk/sona-go/commit/9a97ac77dc83c1043021a1ea3eee46f97067bd01))
* **i18n:** Apple's wording for today's concerts and library copy ([#86](https://github.com/coredev-uk/sona-go/issues/86)) ([302b446](https://github.com/coredev-uk/sona-go/commit/302b446d6d184494b2543a42e9936f2a3ed95d84))
* **motion:** motion artwork never drawn from a rendition narrower than its box ([#180](https://github.com/coredev-uk/sona-go/issues/180)) ([c40540a](https://github.com/coredev-uk/sona-go/commit/c40540a144b0267607cf3cf0a6f20f90a13bdfc2))
* **player:** bar parity and shortcut hints ([#128](https://github.com/coredev-uk/sona-go/issues/128)) ([7111772](https://github.com/coredev-uk/sona-go/commit/7111772d35cd760a516033a553dd65d1da488e63))
* **player:** lyric sheet fades blend into the cover's wash ([#141](https://github.com/coredev-uk/sona-go/issues/141)) ([f2acf97](https://github.com/coredev-uk/sona-go/commit/f2acf9734694cfedc99a3a28ebdd94ac428f54a1))
* **player:** the lyrics screen shows again ([#139](https://github.com/coredev-uk/sona-go/issues/139)) ([2dd0c6a](https://github.com/coredev-uk/sona-go/commit/2dd0c6ac23cc05876152d1f16f8e428f767990b5))
* **playlist:** Apple's generated covers show again; the picker offers Apple's own designs ([#179](https://github.com/coredev-uk/sona-go/issues/179)) ([d3dcd73](https://github.com/coredev-uk/sona-go/commit/d3dcd73ff7ceeef2574f5e21e41dab351ab05064))
* **playlist:** edits hand their answers to the main thread in order ([#164](https://github.com/coredev-uk/sona-go/issues/164)) ([2bc0387](https://github.com/coredev-uk/sona-go/commit/2bc03870a7c811896cfc04e82bf670309b5738ff))
* **probe:** read the window's size when sampling covers ([#157](https://github.com/coredev-uk/sona-go/issues/157)) ([e7d7381](https://github.com/coredev-uk/sona-go/commit/e7d738109f1f03bdf179fbfb7bf514ee6354c123))
* **search:** right-click a result row opens its menu ([#131](https://github.com/coredev-uk/sona-go/issues/131)) ([4151b66](https://github.com/coredev-uk/sona-go/commit/4151b661e60ef6c27025fdc22cdf7a5ba8e55816))
* **settings:** announce settings:changed in save order ([#148](https://github.com/coredev-uk/sona-go/issues/148)) ([#165](https://github.com/coredev-uk/sona-go/issues/165)) ([4aae348](https://github.com/coredev-uk/sona-go/commit/4aae348b7608db2a3ed36516db6fd66ad21418c8))
* **settings:** the update download bar fits beside its actions ([#197](https://github.com/coredev-uk/sona-go/issues/197)) ([54037e8](https://github.com/coredev-uk/sona-go/commit/54037e8e9bad43654e8a5b63dd8140dc1be8587f))
* stop two tests failing at random (reminder loop, artwork loader) ([#168](https://github.com/coredev-uk/sona-go/issues/168)) ([b2069c0](https://github.com/coredev-uk/sona-go/commit/b2069c0964e25f9de7bd494a0565016ef02ea9f9))
* **ui:** fade every page change, so For you no longer slides ([#181](https://github.com/coredev-uk/sona-go/issues/181)) ([f7f8b3d](https://github.com/coredev-uk/sona-go/commit/f7f8b3dde4987c18e59874904fa38092ffda69c8))
* **ui:** parity polish ([#129](https://github.com/coredev-uk/sona-go/issues/129)) ([279ecb7](https://github.com/coredev-uk/sona-go/commit/279ecb7dd127ca837bcdd01530495c12081203da))
* **ui:** phase 2 integration ([#115](https://github.com/coredev-uk/sona-go/issues/115)) ([62091da](https://github.com/coredev-uk/sona-go/commit/62091da4d9cab6a3e35e827fce2305e46c7d26c0))
* **ui:** phase 3 leftovers ([#124](https://github.com/coredev-uk/sona-go/issues/124)) ([1361865](https://github.com/coredev-uk/sona-go/commit/1361865dc7b1a213ced00f069acf6c691755aaf8))
* **ui:** volume slider width and library sort controls jump ([#138](https://github.com/coredev-uk/sona-go/issues/138)) ([d85121f](https://github.com/coredev-uk/sona-go/commit/d85121f95f636905fd92fcac6449d0820abd9ede))
* **vm:** Sona's vendor hash after the go.mod changes of [#149](https://github.com/coredev-uk/sona-go/issues/149) and [#158](https://github.com/coredev-uk/sona-go/issues/158) ([#170](https://github.com/coredev-uk/sona-go/issues/170)) ([901b4db](https://github.com/coredev-uk/sona-go/commit/901b4db462e5cf0a73bf1322af8a784ea85e2567))


### Performance

* **memory:** bound the cover cache by bytes, skip NVDEC for motion artwork, release the web player bundle, report memory per process ([#163](https://github.com/coredev-uk/sona-go/issues/163)) ([b090166](https://github.com/coredev-uk/sona-go/commit/b090166240723991f7d14ede36e12a670ea05468))
* **ui:** parallel artwork decoding and prefetch ([#144](https://github.com/coredev-uk/sona-go/issues/144)) ([a536fb7](https://github.com/coredev-uk/sona-go/commit/a536fb7c84be1b21e75aef77535c0d29a6f3375a))


### Reverts

* ci(release): rc notes list the changes since the previous rc ([#200](https://github.com/coredev-uk/sona-go/issues/200)) ([0fc7d2b](https://github.com/coredev-uk/sona-go/commit/0fc7d2bcbb20ec4469f0335e3980bc3216f99e38))


### Refactoring

* MyGo native UI replaces Wails and React ([#93](https://github.com/coredev-uk/sona-go/issues/93)) ([3b47dc5](https://github.com/coredev-uk/sona-go/commit/3b47dc5dfd3c60516114becba51b2fa88b2e4bbc))

## [0.2.0](https://github.com/coredev-uk/sona-go/compare/v0.1.0...v0.2.0) (2026-11-03)


### Features

* **account:** an account menu at the top right ([#49](https://github.com/coredev-uk/sona-go/issues/49)) ([eddb2d8](https://github.com/coredev-uk/sona-go/commit/eddb2d83be13b4bb58f78eae410108a4a1b1ad78))
* **album:** artwork colours and fuller album information ([#62](https://github.com/coredev-uk/sona-go/issues/62)) ([bda4282](https://github.com/coredev-uk/sona-go/commit/bda4282dd61eb148c35ec9159091a35e210fbe38))
* **album:** redesign the Album page ([#57](https://github.com/coredev-uk/sona-go/issues/57)) ([cf0ab07](https://github.com/coredev-uk/sona-go/commit/cf0ab078d9914a74703aebfc65c7cc8f6d373851))
* **albums:** pre-release albums: release date, reminders and pre-add ([#25](https://github.com/coredev-uk/sona-go/issues/25)) ([70ef2d5](https://github.com/coredev-uk/sona-go/commit/70ef2d55728d94c55ff6e8b61e269147c2649336))
* **artist:** redesign the Artist page ([#59](https://github.com/coredev-uk/sona-go/issues/59)) ([0d8820f](https://github.com/coredev-uk/sona-go/commit/0d8820f22baeea0224c74cb8250cdf706df12202))
* **artwork:** mosaic covers for playlists without artwork ([#72](https://github.com/coredev-uk/sona-go/issues/72)) ([523854f](https://github.com/coredev-uk/sona-go/commit/523854f0591c54874ddf12c02fc8ccd7f373eb4c))
* **browse:** "more to explore" links as buttons ([#79](https://github.com/coredev-uk/sona-go/issues/79)) ([e28ede5](https://github.com/coredev-uk/sona-go/commit/e28ede523b41306b79dd9dcd40f68a0cd269c39b))
* **cli:** control the running Sona from the command line ([#21](https://github.com/coredev-uk/sona-go/issues/21)) ([85243d9](https://github.com/coredev-uk/sona-go/commit/85243d9aac2fb14e3f0bd0c6647f5274711e4321))
* **concerts:** concerts near you, in the app ([#81](https://github.com/coredev-uk/sona-go/issues/81)) ([288cfae](https://github.com/coredev-uk/sona-go/commit/288cfae426a56f5663406056352a3415aa118b5c))
* **credits:** artist pictures and initials in song credits ([#74](https://github.com/coredev-uk/sona-go/issues/74)) ([cd39517](https://github.com/coredev-uk/sona-go/commit/cd395171ba8f796d06f9b343816e5a8305e6053b))
* **discover:** redesign the Discover page ([#55](https://github.com/coredev-uk/sona-go/issues/55)) ([4603744](https://github.com/coredev-uk/sona-go/commit/46037442bd155e93e156815929c5febdbe81c1d2))
* **discover:** rotate the lead featured item ([#78](https://github.com/coredev-uk/sona-go/issues/78)) ([4c0f25d](https://github.com/coredev-uk/sona-go/commit/4c0f25df707828a4b2c715d0bed909375dd16f84))
* **home:** redesign the Home page ([#56](https://github.com/coredev-uk/sona-go/issues/56)) ([6ad2da4](https://github.com/coredev-uk/sona-go/commit/6ad2da4a94877fa359786d0fe78ffbb69f61ce99))
* **i18n:** copy pass towards Apple's wording ([#82](https://github.com/coredev-uk/sona-go/issues/82)) ([98dcd35](https://github.com/coredev-uk/sona-go/commit/98dcd353986601485ed695dee0b626eb11cfe49d))
* **i18n:** error codes, a Go catalogue, Apple content language and a Language setting ([#54](https://github.com/coredev-uk/sona-go/issues/54)) ([8ca7143](https://github.com/coredev-uk/sona-go/commit/8ca71439e0c49c3ba9235e07dfcbba35f4943c3d))
* **i18n:** fetch Apple Music's translations at runtime ([#48](https://github.com/coredev-uk/sona-go/issues/48)) ([5a64e44](https://github.com/coredev-uk/sona-go/commit/5a64e44e9492313ecd32f44cd663a73a649ba166))
* **i18n:** map Sona's messages to Apple Music's string keys ([#17](https://github.com/coredev-uk/sona-go/issues/17)) ([2fd8952](https://github.com/coredev-uk/sona-go/commit/2fd89525357c7cb697e02dcaa1990499eb6d0411))
* **library:** an artists overview with search ([#77](https://github.com/coredev-uk/sona-go/issues/77)) ([1fb679f](https://github.com/coredev-uk/sona-go/commit/1fb679f103966d4b0ad8e9b51156ce06ef97f2dc))
* **library:** filter albums by Albums and Singles & EPs ([#76](https://github.com/coredev-uk/sona-go/issues/76)) ([5aa74b3](https://github.com/coredev-uk/sona-go/commit/5aa74b3b76bff9805b4985f2dbdf4c5f28c3ba9e))
* **lyrics:** lyrics from Apple, Musixmatch, LRCLIB and NetEase ([#53](https://github.com/coredev-uk/sona-go/issues/53)) ([536dc02](https://github.com/coredev-uk/sona-go/commit/536dc0256be3c32b0ca3bf8cb593153187ce6e6d))
* **lyrics:** redesign the lyrics view, full-screen lyrics and the mini player line ([#61](https://github.com/coredev-uk/sona-go/issues/61)) ([edd772b](https://github.com/coredev-uk/sona-go/commit/edd772b8102fa8791408def589773ec14488e698))
* **lyrics:** word-by-word lyrics with duets, background vocals and translations ([#58](https://github.com/coredev-uk/sona-go/issues/58)) ([409c5db](https://github.com/coredev-uk/sona-go/commit/409c5db09d13cd3d96ab257e81379a5418839070))
* **onboarding:** a first-run setup ([#40](https://github.com/coredev-uk/sona-go/issues/40)) ([fc489ee](https://github.com/coredev-uk/sona-go/commit/fc489eedb2fdd41cae29920a1fd8c696de263fce))
* **onboarding:** a full-screen first-run setup ([#50](https://github.com/coredev-uk/sona-go/issues/50)) ([8fa9243](https://github.com/coredev-uk/sona-go/commit/8fa9243548d3ab0f8441c3fdc5a0a081c63ca33c))
* **onboarding:** first-run setup as an overlay over the app ([#63](https://github.com/coredev-uk/sona-go/issues/63)) ([4a39c3f](https://github.com/coredev-uk/sona-go/commit/4a39c3fa94771c6efe6bde632351e92f75319b6a))
* **playback:** ask for realtime priority for the audio thread ([#27](https://github.com/coredev-uk/sona-go/issues/27)) ([4c08828](https://github.com/coredev-uk/sona-go/commit/4c08828ff6d5f5261799bf04fe3a759bba12a396))
* **player:** live stations disable controls that don't apply ([#85](https://github.com/coredev-uk/sona-go/issues/85)) ([d70f283](https://github.com/coredev-uk/sona-go/commit/d70f2839eca59ab139edd910558afd63b8298cad))
* **player:** redesign the Now Playing view ([#84](https://github.com/coredev-uk/sona-go/issues/84)) ([4c29867](https://github.com/coredev-uk/sona-go/commit/4c298672c26fd55f09ad531546c96d01cadf2925))
* **queue:** play next and move to end from the queue ([#83](https://github.com/coredev-uk/sona-go/issues/83)) ([38f4a93](https://github.com/coredev-uk/sona-go/commit/38f4a933f00f533bf10247f6aa5582c385dd3b87))
* **queue:** show the AutoPlay songs ahead of time ([#18](https://github.com/coredev-uk/sona-go/issues/18)) ([68e1bcc](https://github.com/coredev-uk/sona-go/commit/68e1bccb75ee54c106249de2dd7653452675c30e))
* **radio:** play local broadcasters ([#44](https://github.com/coredev-uk/sona-go/issues/44)) ([9e8dd46](https://github.com/coredev-uk/sona-go/commit/9e8dd460d20c0e0f5660ee8da2f8dab8eb09cfe4))
* **radio:** play stations without a schedule directly; live track info for local broadcasters ([#80](https://github.com/coredev-uk/sona-go/issues/80)) ([2d7c073](https://github.com/coredev-uk/sona-go/commit/2d7c07390d3943e5702dabf2b4f1b2e5a694fc03))
* **radio:** redesign the Radio page ([#60](https://github.com/coredev-uk/sona-go/issues/60)) ([2f66d4f](https://github.com/coredev-uk/sona-go/commit/2f66d4f389fe23a4eae43235984bb63bfe5aa9ef))
* **settings:** redesign the Settings page ([#68](https://github.com/coredev-uk/sona-go/issues/68)) ([e945c8b](https://github.com/coredev-uk/sona-go/commit/e945c8bb6dedbc5ccdb5a33db1dee597f08c1115))
* **shell:** mouse back and forward buttons navigate ([#70](https://github.com/coredev-uk/sona-go/issues/70)) ([63def4e](https://github.com/coredev-uk/sona-go/commit/63def4e65bb21d9448d4e1fb205f693444925e94))
* **sidebar:** Made for You and Apple Music playlist sections ([#73](https://github.com/coredev-uk/sona-go/issues/73)) ([70a5ca9](https://github.com/coredev-uk/sona-go/commit/70a5ca9aec4df994cc6d4e50f7efc42bb02b6503))
* **video:** play music videos ([#65](https://github.com/coredev-uk/sona-go/issues/65)) ([5512a08](https://github.com/coredev-uk/sona-go/commit/5512a0891eda1f21455378b05121a2b7e8eb4c81))


### Bug Fixes

* **artwork:** request editorial art at its own aspect ratio ([#19](https://github.com/coredev-uk/sona-go/issues/19)) ([294444a](https://github.com/coredev-uk/sona-go/commit/294444ad77cd4a33662bf417eba704d5a6b7d24b))
* **auth:** bind the loopback sign-in to the browser that started it ([#22](https://github.com/coredev-uk/sona-go/issues/22)) ([cf40041](https://github.com/coredev-uk/sona-go/commit/cf400419270f97657f9a433675d718ff54ea3bb0))
* **auth:** don't hold the window for a slow keyring ([#20](https://github.com/coredev-uk/sona-go/issues/20)) ([286d3e5](https://github.com/coredev-uk/sona-go/commit/286d3e5ace462389796a8fdc3e957b5a15278bb6))
* **deps:** update dependency @tanstack/react-virtual to ^3.14.24 ([#5](https://github.com/coredev-uk/sona-go/issues/5)) ([995bf88](https://github.com/coredev-uk/sona-go/commit/995bf88d901bbc9fe4e82f80d1b33fb9fa2100c3))
* **deps:** update dependency lucide-react to ^1.64.0 ([#8](https://github.com/coredev-uk/sona-go/issues/8)) ([c940cfd](https://github.com/coredev-uk/sona-go/commit/c940cfdf983a7254c2f5cb7c2406775aa7c67a21))
* **deps:** update module golang.org/x/tools to v0.52.0 ([#15](https://github.com/coredev-uk/sona-go/issues/15)) ([f503ad0](https://github.com/coredev-uk/sona-go/commit/f503ad0f32c37fc8bcdf67cb6a60b7e3d9840508))
* deterministic playback, lyrics and library tests ([#71](https://github.com/coredev-uk/sona-go/issues/71)) ([90e4b57](https://github.com/coredev-uk/sona-go/commit/90e4b57227ef40a53c3b03b99644f5a7bff292d0))
* **devtools:** typing in the inspector console works ([#75](https://github.com/coredev-uk/sona-go/issues/75)) ([3c2408a](https://github.com/coredev-uk/sona-go/commit/3c2408a543273eb5aaf2b14c3048dfb07181bb9b))
* **discover:** banner art fills its frame ([#32](https://github.com/coredev-uk/sona-go/issues/32)) ([809810d](https://github.com/coredev-uk/sona-go/commit/809810df4a5abdb79e05373808561e1d147902d6))
* **library:** the sync indicator stops when the sync is done ([#43](https://github.com/coredev-uk/sona-go/issues/43)) ([eb269c5](https://github.com/coredev-uk/sona-go/commit/eb269c5361fa329df85936fe27931a521a06ce70))
* **motion:** play motion artwork, and show why when it can't ([#29](https://github.com/coredev-uk/sona-go/issues/29)) ([2230356](https://github.com/coredev-uk/sona-go/commit/22303563559d1b6fc580321d93741d24f7e82369))
* **packaging:** check the man page and completions in the packages' file lists ([#31](https://github.com/coredev-uk/sona-go/issues/31)) ([1bd4abd](https://github.com/coredev-uk/sona-go/commit/1bd4abd7eb663ea7451840d8dcc359ce9d3c35f2))
* **playback:** moving the output right after a pause no longer fails ([#28](https://github.com/coredev-uk/sona-go/issues/28)) ([afdec9c](https://github.com/coredev-uk/sona-go/commit/afdec9ce516d9a3e54b46f035604307d534ac73c))
* **playlists:** show collaborators and who added each song ([#52](https://github.com/coredev-uk/sona-go/issues/52)) ([d5089f9](https://github.com/coredev-uk/sona-go/commit/d5089f9286d150291a823739c24a5d89943fd50b))
* **radio:** sharp artwork in station schedules ([#37](https://github.com/coredev-uk/sona-go/issues/37)) ([c75784c](https://github.com/coredev-uk/sona-go/commit/c75784c6cc17da806c3c9cb23102f8151af1d588))
* **shell:** blank mini player when GTK has no font DPI; upstream WebKitGTK report ([#23](https://github.com/coredev-uk/sona-go/issues/23)) ([b80c932](https://github.com/coredev-uk/sona-go/commit/b80c9320bd3add8b9d5bc4d989ba2b7e7be02d8b))
* **sidebar:** list folders before playlists at every level ([#11](https://github.com/coredev-uk/sona-go/issues/11)) ([0130804](https://github.com/coredev-uk/sona-go/commit/0130804e981e65ffb2389ba26aeef4bb0215edc7))
* **sidebar:** show pinned playlists only under Pinned ([#47](https://github.com/coredev-uk/sona-go/issues/47)) ([b0a8537](https://github.com/coredev-uk/sona-go/commit/b0a853787f25f4b5b187f338cea27fecff1a4c39))


### Performance

* cheaper artwork painting, measured with cmd/uiprobe ([#30](https://github.com/coredev-uk/sona-go/issues/30)) ([1bf2998](https://github.com/coredev-uk/sona-go/commit/1bf2998f3448b659a305062d6ddbea9a1b4091a6))
* **motion:** measure motion artwork's CPU cost; pause videos under Now Playing ([#38](https://github.com/coredev-uk/sona-go/issues/38)) ([8829bf9](https://github.com/coredev-uk/sona-go/commit/8829bf9e126a49c3447e349059f87941dd5503d0))
