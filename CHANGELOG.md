# Changelog

# [0.8.0](https://github.com/goliatone/go-notifications/compare/v0.7.0...v0.8.0) - (2026-01-03)

## <!-- 1 -->🐛 Bug Fixes

- Logger definition should match required api ([6776e0e](https://github.com/goliatone/go-notifications/commit/6776e0e7fcb16ad451e6462b8c80d9d0ce4b9662))  - (goliatone)

## <!-- 13 -->📦 Bumps

- Bump version: v0.8.0 ([44040a9](https://github.com/goliatone/go-notifications/commit/44040a9b5d77dc9f85d1187962e4bcc9006992cf))  - (goliatone)

## <!-- 16 -->➕ Add

- Use new default logger ([80d4b12](https://github.com/goliatone/go-notifications/commit/80d4b12c0e2cffc380832efbadd701a7c97c2d57))  - (goliatone)
- Basic logger implementation ([1aa2725](https://github.com/goliatone/go-notifications/commit/1aa27257fbec86bc2d6abf982700731411e2655a))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.7.0 ([ba35c63](https://github.com/goliatone/go-notifications/commit/ba35c63a50869c99623cd7511261685d3255b379))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update example ([43c4495](https://github.com/goliatone/go-notifications/commit/43c449511ab98d96f4ab97c13f091d9ef7a84036))  - (goliatone)
- Update tests ([7955994](https://github.com/goliatone/go-notifications/commit/7955994cf89f40f3f7105db91678c95192ca5b3f))  - (goliatone)

# [0.7.0](https://github.com/goliatone/go-notifications/compare/v0.6.0...v0.7.0) - (2026-01-02)

## <!-- 1 -->🐛 Bug Fixes

- Ensure we have send message metadata ([6091d9b](https://github.com/goliatone/go-notifications/commit/6091d9bf645efeb31beab8857dda2ddf69f48fb0))  - (goliatone)
- Update github workflow to enable release ([a71dc52](https://github.com/goliatone/go-notifications/commit/a71dc52cb82564bc3cd1a46452d8dca2b5040b4b))  - (goliatone)
- Lintin error ([84f28e9](https://github.com/goliatone/go-notifications/commit/84f28e9a092fb3874552e06246b3ae5954ba5c84))  - (goliatone)
- Name repo ([0c2bca9](https://github.com/goliatone/go-notifications/commit/0c2bca9dfd40b276c7ce1d938f5be711ed90a3c3))  - (goliatone)
- Increase timeout for go lint ([f2c9c3e](https://github.com/goliatone/go-notifications/commit/f2c9c3e2f027e117ecc7f98085ea9023cf2aa893))  - (goliatone)
- Remove unused task ([617bb22](https://github.com/goliatone/go-notifications/commit/617bb2212035d64f61d38aa3f39cc6d0165e10e6))  - (goliatone)
- Update smtp to use fmt.Fprintf ([fbc9016](https://github.com/goliatone/go-notifications/commit/fbc90162da71717b497cfaab2e8148dce045b849))  - (goliatone)

## <!-- 13 -->📦 Bumps

- Bump version: v0.7.0 ([37cc2aa](https://github.com/goliatone/go-notifications/commit/37cc2aa88fa3b018dbc8bb8bae7ca71586d5f8ed))  - (goliatone)

## <!-- 16 -->➕ Add

- Html2text to generate non markup input ([9a6d714](https://github.com/goliatone/go-notifications/commit/9a6d71425201b845b2acff14309bdbf45dc5bf66))  - (goliatone)
- Support for html body and text body for SMTP messages ([eeb8816](https://github.com/goliatone/go-notifications/commit/eeb8816290b379104f0fe0738982bf53a6f08149))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.6.0 ([2e2476e](https://github.com/goliatone/go-notifications/commit/2e2476e65a4833fccb33939a75cbec8301dd809d))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update tests ([38c1a5a](https://github.com/goliatone/go-notifications/commit/38c1a5a04c592795af70b5980478ab6a225b10ae))  - (goliatone)
- Update deps ([d9093fc](https://github.com/goliatone/go-notifications/commit/d9093fc16751ecc9d20962a5279d7a9a75b4addb))  - (goliatone)
- Update github workflow ([c0e2ac3](https://github.com/goliatone/go-notifications/commit/c0e2ac3da02e8f969fb93c677df5737cd09f6604))  - (goliatone)
- Update example ([9afd369](https://github.com/goliatone/go-notifications/commit/9afd3694ff10fa9a289c389d8b53f0d42287ad2d))  - (goliatone)

# [0.6.0](https://github.com/goliatone/go-notifications/compare/v0.5.0...v0.6.0) - (2025-12-23)

## <!-- 1 -->🐛 Bug Fixes

- Template service should handle data races ([ef99de8](https://github.com/goliatone/go-notifications/commit/ef99de84adb8ec380d643c7f971b140d2d5b9925))  - (goliatone)

## <!-- 13 -->📦 Bumps

- Bump version: v0.6.0 ([da49770](https://github.com/goliatone/go-notifications/commit/da49770b503a3c808525a5a343f1750e7fb9f574))  - (goliatone)

## <!-- 16 -->➕ Add

- On ready notifier should handle attachment URLs ([1ce47c7](https://github.com/goliatone/go-notifications/commit/1ce47c7928ff910dc4fac57fdd8761b4a60c55eb))  - (goliatone)
- Notifier manager handle attachments ([24a78e7](https://github.com/goliatone/go-notifications/commit/24a78e7aa21cd4ae1a0da8c0dbda1441c9da3164))  - (goliatone)
- Whatsapp adapter handle attachments ([42a7101](https://github.com/goliatone/go-notifications/commit/42a710199dfe00ae9f171f476cc739be6eaf6b34))  - (goliatone)
- Twilio adapter handle attachments ([c9ff898](https://github.com/goliatone/go-notifications/commit/c9ff8984c74fdd704b97a1e6ac3645e78529882a))  - (goliatone)
- Telegram adapter handle attachments ([974732d](https://github.com/goliatone/go-notifications/commit/974732d806882c73b9a2dd85a6a1d638ba02be0a))  - (goliatone)
- Slack adapter handle attachments ([0ee66d0](https://github.com/goliatone/go-notifications/commit/0ee66d0655633a2597bb9c2be82de347c84c9c95))  - (goliatone)
- Attachment resolution ([1f5b6b0](https://github.com/goliatone/go-notifications/commit/1f5b6b0bbbc6e2c9fff2ee7c671d9dae7c683a2c))  - (goliatone)
- Expose Attachmetns in dispatcher ([837f95c](https://github.com/goliatone/go-notifications/commit/837f95cefc1882911170f734daa04d9c34d92d94))  - (goliatone)
- Expose Attachmetns in di container ([6697a04](https://github.com/goliatone/go-notifications/commit/6697a049a024dc3b29e99f301c127d6d2864d679))  - (goliatone)
- Attachment resolver ([c143cf0](https://github.com/goliatone/go-notifications/commit/c143cf0a0600538476cffafd8ae95eb3c6841dc7))  - (goliatone)
- Support attachments ([5dd56f5](https://github.com/goliatone/go-notifications/commit/5dd56f5ec4d9ab5107e73c4f3e915eda6eb981b6))  - (goliatone)
- Attachment support for service ([d97cea4](https://github.com/goliatone/go-notifications/commit/d97cea4de074b30b2c6b6a8016ab3231e929b74e))  - (goliatone)
- Attachment support ([2cee70c](https://github.com/goliatone/go-notifications/commit/2cee70c69aa912c5680954fb5a3a86186fab282c))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.5.0 ([197c6e6](https://github.com/goliatone/go-notifications/commit/197c6e6e433a1aa8b3230504e8930c9816c30e52))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update readme ([6f340ad](https://github.com/goliatone/go-notifications/commit/6f340ad6ec6256f7b5a5f9e93deadab5904d4477))  - (goliatone)
- Udpate readme ([44b7203](https://github.com/goliatone/go-notifications/commit/44b7203243c1b8e81fb2c3176b7e9fc5e85dd8f0))  - (goliatone)
- Udpate test ([b870595](https://github.com/goliatone/go-notifications/commit/b870595db554cf68460125af56194e56156ad387))  - (goliatone)
- Update docs ([b2c2d86](https://github.com/goliatone/go-notifications/commit/b2c2d862efa940eaf3ba5d15494bd161c257f66f))  - (goliatone)

# [0.5.0](https://github.com/goliatone/go-notifications/compare/v0.4.0...v0.5.0) - (2025-12-16)

## <!-- 13 -->📦 Bumps

- Bump version: v0.5.0 ([a92add6](https://github.com/goliatone/go-notifications/commit/a92add6c810ccaa1442fb686d644c6da29a9867b))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.4.0 ([0056952](https://github.com/goliatone/go-notifications/commit/00569522be47b0b8aff708933b36b23dab5e2658))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update go.mod ([39b4b1f](https://github.com/goliatone/go-notifications/commit/39b4b1f5015fec0069b5bcd5551b318aa085769e))  - (goliatone)
- Update version ([3d28dcb](https://github.com/goliatone/go-notifications/commit/3d28dcbbe1df12327cb9e0f3d38421f4ed778818))  - (goliatone)
- Update gitignore ([4c1fb01](https://github.com/goliatone/go-notifications/commit/4c1fb014262479da9e611140a770195bbf7410d1))  - (goliatone)

# [0.4.0](https://github.com/goliatone/go-notifications/compare/v0.3.0...v0.4.0) - (2025-12-13)

## <!-- 16 -->➕ Add

- Activity manager ([7e480d1](https://github.com/goliatone/go-notifications/commit/7e480d107d8a2caee9f31fdc2df00cd60bb21d29))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.3.0 ([4f8f2de](https://github.com/goliatone/go-notifications/commit/4f8f2def3b905b64bd05b0af364ae84990fa13bd))  - (goliatone)

# [0.3.0](https://github.com/goliatone/go-notifications/compare/v0.2.0...v0.3.0) - (2025-12-13)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.2.0 ([b26c281](https://github.com/goliatone/go-notifications/commit/b26c28150a7cd774160c402e6eb1d14e4249d1d6))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update deps ([1b4bbd3](https://github.com/goliatone/go-notifications/commit/1b4bbd3663dd9398b3d665a1ffbd07bd9e5ea5cc))  - (goliatone)

# [0.2.0](https://github.com/goliatone/go-notifications/compare/v0.1.0...v0.2.0) - (2025-12-02)

## <!-- 16 -->➕ Add

- Activity hooks ([63146ae](https://github.com/goliatone/go-notifications/commit/63146aef4ce398e0a840c4c41e7b297c9bb999a3))  - (goliatone)
- Activity  adapter ([10d727d](https://github.com/goliatone/go-notifications/commit/10d727d08f910080dd01af07a9cdcdb98f0f893a))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.1.0 ([c8c4357](https://github.com/goliatone/go-notifications/commit/c8c4357f9a29341cce0c9e91d6cb0a45f2973e81))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update readme ([3879d09](https://github.com/goliatone/go-notifications/commit/3879d09895fbba7bd5f519e4db0a931992a0f857))  - (goliatone)
- Update deps ([be8152e](https://github.com/goliatone/go-notifications/commit/be8152ef2e314bb5b8039cdec870d2ad89a5e0b9))  - (goliatone)

# [0.1.0](https://github.com/goliatone/go-notifications/tree/v0.1.0) - (2025-11-25)

## <!-- 1 -->🐛 Bug Fixes

- Normalize payload ([0834b99](https://github.com/goliatone/go-notifications/commit/0834b99744c7012653435e09c6f3a8677d859b34))  - (goliatone)
- Test setup ([9a2dd5b](https://github.com/goliatone/go-notifications/commit/9a2dd5bded4cd53c3522caf657f4edbedc188fb7))  - (goliatone)
- Template type ([46c2c39](https://github.com/goliatone/go-notifications/commit/46c2c39c76c830ae3805ab7f2ebd2b9cd5c12b1c))  - (goliatone)
- Apply override ([a0d291e](https://github.com/goliatone/go-notifications/commit/a0d291eb2b2e264ee9f637b2a6e56a3756af7c79))  - (goliatone)
- Export feature ([3953ea0](https://github.com/goliatone/go-notifications/commit/3953ea03c4fd423188b56c195e65dd3cbbb0ccb9))  - (goliatone)
- Example style ([be35f8c](https://github.com/goliatone/go-notifications/commit/be35f8cfa4d13087d914c2438b513ba43f5cfdc1))  - (goliatone)
- No err found ([2dd67bb](https://github.com/goliatone/go-notifications/commit/2dd67bb2369d1c1a97bf95d7b8222c44cc4441ac))  - (goliatone)
- Normalize channel key ([9181374](https://github.com/goliatone/go-notifications/commit/9181374cd295c5096f2d73614682d00e01a2dfe5))  - (goliatone)
- Lintin issues ([e71b1fb](https://github.com/goliatone/go-notifications/commit/e71b1fb773c2c31b61091cf3181f34af50c16ef0))  - (goliatone)
- Telegram message format ([47be045](https://github.com/goliatone/go-notifications/commit/47be04545af9544858b45ebf98c27e580419a0c1))  - (goliatone)
- Example setup ([76f2991](https://github.com/goliatone/go-notifications/commit/76f299195ae41ead052ac821972843cd676487ea))  - (goliatone)
- Ci workflow ([d8f862f](https://github.com/goliatone/go-notifications/commit/d8f862f53df05471248973585d35e36cefa88016))  - (goliatone)
- Example display ([018f6ed](https://github.com/goliatone/go-notifications/commit/018f6edaf7fcb9ef7bd98d8df3dd7286fb86eedf))  - (goliatone)
- Code linting issues ([90e3dbd](https://github.com/goliatone/go-notifications/commit/90e3dbd27d2c25bd306623a15f3531e4feb28391))  - (goliatone)
- Improve dispatcher logging ([c237ec0](https://github.com/goliatone/go-notifications/commit/c237ec0d36fb35b44bc2f729c559c3d20e547c5a))  - (goliatone)

## <!-- 16 -->➕ Add

- Export help ([f3dc723](https://github.com/goliatone/go-notifications/commit/f3dc72302f169f1431a80fc58da10e58bf940eaa))  - (goliatone)
- Secret store ([c8fad43](https://github.com/goliatone/go-notifications/commit/c8fad4382bcef7a5d8aaadb4f5456bb03b89d022))  - (goliatone)
- Secrets for auth ([1f32e44](https://github.com/goliatone/go-notifications/commit/1f32e449900a3500a9bbc2a6606cb2c489f4f4aa))  - (goliatone)
- Update example to use adapters ([84c0c75](https://github.com/goliatone/go-notifications/commit/84c0c75e9fc996feb6b2ae5df2b9f9f876ad45aa))  - (goliatone)
- Preferencs setup ([3aed5f0](https://github.com/goliatone/go-notifications/commit/3aed5f0f47a5f57465a1602af7c213bd39a536ba))  - (goliatone)
- Webhook channel adapter ([37da4eb](https://github.com/goliatone/go-notifications/commit/37da4eb6c521452e858fe39ec550e230e3288b61))  - (goliatone)
- Slack channel adapter ([289c514](https://github.com/goliatone/go-notifications/commit/289c51446512e9432ecbd84934e3a938a7aacd05))  - (goliatone)
- Firebase FCM channel adapter ([1088558](https://github.com/goliatone/go-notifications/commit/1088558d0ffeb28e8450dc8190cf43f6260ad9f6))  - (goliatone)
- SNS channel adapter ([6f180d1](https://github.com/goliatone/go-notifications/commit/6f180d16a2486335c51f8acb4da0a8423b25522a))  - (goliatone)
- SES channel adapter ([e048989](https://github.com/goliatone/go-notifications/commit/e04898937bacbc3f7a639dc1d569240aaef668d2))  - (goliatone)
- Dry run option for testing ([4b66f28](https://github.com/goliatone/go-notifications/commit/4b66f287a133239eadbe00d3d05602370e9d3fc8))  - (goliatone)
- Mailgun channel ([8f6f698](https://github.com/goliatone/go-notifications/commit/8f6f6981f33ffa1c93eb671bdd0e90849046bfaf))  - (goliatone)
- Implement sendgrid channel ([ef01543](https://github.com/goliatone/go-notifications/commit/ef01543b2f7c767fb99e88514dc6e486678f102c))  - (goliatone)
- Implement console channel ([c9ce865](https://github.com/goliatone/go-notifications/commit/c9ce865f51dea303fc99a157c9c8e921ca64619e))  - (goliatone)
- Implementation for Twilio ([4b80b29](https://github.com/goliatone/go-notifications/commit/4b80b2915e42888e63d473cc2417a9631f94b2cf))  - (goliatone)
- Implementation for SMTP ([09581c6](https://github.com/goliatone/go-notifications/commit/09581c6807519f871f1acc45891f955e0b87ca20))  - (goliatone)
- Container to di ([4368670](https://github.com/goliatone/go-notifications/commit/4368670f5d6c48083d68e0d2c25771d4ef99ec42))  - (goliatone)
- Go-cms adapters ([12c9db8](https://github.com/goliatone/go-notifications/commit/12c9db8adf1e329b15aaeee9bca0ea877a7ec6a9))  - (goliatone)
- New interface methods ([c953b6e](https://github.com/goliatone/go-notifications/commit/c953b6e289d2161931b2eee66bc1f5d0ffd9460e))  - (goliatone)
- Notifier module ([c139ec9](https://github.com/goliatone/go-notifications/commit/c139ec9cc816ea52fb0635d9e24f987be3ee98fd))  - (goliatone)
- Commands to manage ([e09b0a9](https://github.com/goliatone/go-notifications/commit/e09b0a9b1381545c50e4eba41e557c802e8a9668))  - (goliatone)
- Di package ([c4107be](https://github.com/goliatone/go-notifications/commit/c4107be79b7c24968f9f8a855300bb03eb2b0e97))  - (goliatone)
- Command catalog ([b1580aa](https://github.com/goliatone/go-notifications/commit/b1580aa7265ac099844e28756933ccd58d0d741c))  - (goliatone)
- Delivery from inbox ([c96cc68](https://github.com/goliatone/go-notifications/commit/c96cc688b70498f458a56790e48346d4b5a7a5db))  - (goliatone)
- Dispatch internal implementation ([8817202](https://github.com/goliatone/go-notifications/commit/8817202c6d57e4264f94e713b40d32f430c20985))  - (goliatone)
- Internal inbox ([706a971](https://github.com/goliatone/go-notifications/commit/706a971a75e3372dcb81b5672ed65bc748cacb03))  - (goliatone)
- Events service ([0c0f725](https://github.com/goliatone/go-notifications/commit/0c0f725d67125fb87158f83d5d0f96540c87d61e))  - (goliatone)
- Events ([4b1b7d2](https://github.com/goliatone/go-notifications/commit/4b1b7d2f47fdb0210c9dd21fecaf97c6c0513ea6))  - (goliatone)
- Inbox ([ee5f028](https://github.com/goliatone/go-notifications/commit/ee5f02889bf44e3adf04d1fac18ab4051bd22894))  - (goliatone)
- Broadcaster interfaces ([92685e2](https://github.com/goliatone/go-notifications/commit/92685e2093f0c860398c6d2f5afe585afaf165a9))  - (goliatone)
- Templates ([c033c05](https://github.com/goliatone/go-notifications/commit/c033c05d08681558008fcf97d6088c19f289badc))  - (goliatone)
- Internal pacakges ([69fca6e](https://github.com/goliatone/go-notifications/commit/69fca6e1047cdcb83dab2031ca5dfbaee2b45a0c))  - (goliatone)
- Preferences package ([431cb9d](https://github.com/goliatone/go-notifications/commit/431cb9d9cd353a44e8352d734999d224a643103a))  - (goliatone)
- Otpions management ([e0de6bd](https://github.com/goliatone/go-notifications/commit/e0de6bdc4b998bbc1df6199f052dfa4fc4bfbff0))  - (goliatone)
- Notifier handler ([c76e476](https://github.com/goliatone/go-notifications/commit/c76e47677d6cb7735d3f3f83f278a664ce37605a))  - (goliatone)
- Package interfaces ([ebe4e00](https://github.com/goliatone/go-notifications/commit/ebe4e0066e77b8c13ca33eaed4a59e7650290daa))  - (goliatone)
- Configuration handler ([8eec75e](https://github.com/goliatone/go-notifications/commit/8eec75eba2bd9f1e3824e480ee81ae10aa5eda1e))  - (goliatone)
- Entities ([c0051ff](https://github.com/goliatone/go-notifications/commit/c0051ff38f7d897399788fb528b82e7232180c63))  - (goliatone)
- Adapters ([fd44e2b](https://github.com/goliatone/go-notifications/commit/fd44e2b2af0edbf487b0d7f4368ef7f461339d0e))  - (goliatone)
- Storage pacakge ([18c5b69](https://github.com/goliatone/go-notifications/commit/18c5b69de27148a6479833b2b0990e43aaab5bb8))  - (goliatone)

## <!-- 2 -->🚜 Refactor

- Rename to onready ([5eaf562](https://github.com/goliatone/go-notifications/commit/5eaf56215d7220df0203e84cd8ee9d584a3260cd))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update gitignore ([82b61e7](https://github.com/goliatone/go-notifications/commit/82b61e7a4520f1efa8ef3a7b9a130041430e53ec))  - (goliatone)
- Update onready ([12a8717](https://github.com/goliatone/go-notifications/commit/12a87171d7284e728eaee9dc88b4b6f0013ddf0f))  - (goliatone)
- Update readme ([5871066](https://github.com/goliatone/go-notifications/commit/58710669b33a345d5dff7773386c683d24d6624e))  - (goliatone)
- Update example ([26ebd5e](https://github.com/goliatone/go-notifications/commit/26ebd5ee8b2aa892db3f148fc454cfe668e457a7))  - (goliatone)
- Clean up tests ([00a7979](https://github.com/goliatone/go-notifications/commit/00a7979141309268fe55a48e1bb162723b460907))  - (goliatone)
- Update tests ([1257db6](https://github.com/goliatone/go-notifications/commit/1257db66f3b34bd35154947f0d795f52647b772f))  - (goliatone)
- Update deps ([93001bb](https://github.com/goliatone/go-notifications/commit/93001bb609f9ff5579616b0b09a5cc4e1f5b1c57))  - (goliatone)
- Update tasks ([4ab9538](https://github.com/goliatone/go-notifications/commit/4ab9538c27535963134fdad75089a3110ae4d60f))  - (goliatone)
- Update docs ([2fd83df](https://github.com/goliatone/go-notifications/commit/2fd83df999a7052bb1afcbc83d129d0c64330bd3))  - (goliatone)
- Add github workflow ([ec8f148](https://github.com/goliatone/go-notifications/commit/ec8f1487aa848fd1d0c87fd04ebeb495a72ca6bb))  - (goliatone)
- Update examples ([c3553b2](https://github.com/goliatone/go-notifications/commit/c3553b244dd41eb1513b9d3e65bd64e55fd66c7d))  - (goliatone)
- Clean up ([0571c72](https://github.com/goliatone/go-notifications/commit/0571c72dcc0cee1b884f86f41aebeeffca16b0c3))  - (goliatone)
- Initial commit ([9a8eb26](https://github.com/goliatone/go-notifications/commit/9a8eb26019ceb07b26fda728351d193e984cd27c))  - (goliatone)


