# TokenHub 老师/学生多租户改造计划（v2，已含 OCR 审查补充点）

目标：老师（team_leader 角色）自带 LLM 渠道、自助管理学生与路由、邀请码注册、管理员按团队定价向老师收费。单分支 `feat/teacher-tenancy` 逐功能 commit（含测试+三语文档），推送 fork `szyhf/TokenHub`。

## Phase 0 准备
- `git remote add fork https://github.com/szyhf/TokenHub`，从 origin/main（7ef9b63）建 `feat/teacher-tenancy`
- 遵守 AGENTS.md：gofmt、golangci-lint 用 ci.yml 固定版本、`git diff --check`

## F1 渠道租户化（后端）`feat(providers): team-scoped tenant-owned provider channels`
- `Provider`/`ProviderResource` 加 `OwnerTeamID`（空=平台渠道）；v7 双方言迁移（照版本 5 模式）+ baseline/manifest 重生成
- `canAdmin` 开放 team_leader 的 provider 写；新文件 `admin_provider_tenancy.go` 统一归属校验
- [H1] 归属校验覆盖全部写入路径：CRUD + `BulkOperateProviderResources` + `ImportProviderResources` + 探活 + 密钥轮换 + egress 配置
- [H3] 老师提交的 BaseURL 继承 strict 出向管控（CIDR 白名单/禁 loopback），测试验证不可绕过
- [M1] 删除渠道时阻止或级联清理引用路由；[L1] 审计事件
- 测试：自有 CRUD ✓ / 跨老师 403 / 平台渠道不可改 / 批量导入路径 403；文档 administrator-guide + team-leader-guide + architecture.md ×3

## F2 租户路由隔离（后端）`feat(routing): team-scoped model routes`
- `ModelRoute` 加 `OwnerTeamID`；建路由时校验仅能引用自有渠道
- [M2] 团队谓词放 store 层 `loadRouteCandidates`，gateway_http.go 只留一行调用；验证 `SelectRouteCandidates` 全部调用方（playground/健康检查）受限
- 测试：老师 A 学生不命中老师 B 渠道 + playground 隔离；文档 ×3

## F3 老师控制台（前端）`feat(console): teacher provider and route management`
- `roleViewAccess` 开放 team_leader 的 providers/routes；全部新文件（provider-editor.tsx 冻结禁改）
- [M4] TS `Provider` 类型加 owner_team_id；管理员列表显示归属徽标
- [M6] README Capabilities 三语各加一条；tx() 同 commit 补 en/ja；UI fixture teacher 场景；文档 ×3

## F4 邀请码注册 `feat(auth): invite-code teacher self-registration`
- `cfg_gateway` 设置：allow_self_registration（默认 false）+ registration_invite_code（secret 保护）
- 公开 GET /api/admin/auth/registration-status + POST /api/admin/auth/register；crypto/subtle 常量时间比较；集群 lease 限速
- [H2] 密码强度校验（公开接口强制）；建团队+建用户+审计单事务
- [M5] 团队命名冲突处理；409 泄露用户名与登录行为一致并文档明示
- 测试：关闭 403/错码 403/冲突 409/弱密码 400/成功/限速；登录页注册入口+RegisterView+翻译；文档 ×3

## F5 老师账单可见性 `feat(billing): team-scoped tenant statements`
- `billing_statements.go:122` team_leader 分支：强制 side=tenant、注入本团队过滤、剥离 margin/provider 行；[L3] CSV 导出同路径
- 前端放开 BillingView team_leader 门控（rate cards 保持 admin-only）；测试镜像 billing_statements_test.go:179；文档 ×3

## F6 按团队定价 `feat(billing): per-team tenant rate cards`
- meteringRateCard 新 kind tenant_team（target=teamID:model，无需改表）；解析 tenant_team → 全局 tenant 回退
- [M3] publish 时校验 teamID 存在；前端价卡加团队维度；测试回退顺序与账单金额；文档 ×3

## 每功能统一门禁
后端 gofmt/go test/go vet/golangci-lint；前端 lint/typecheck/test/build；`node --test tools/*.test.mjs` + check-doc/ui-translations/source-lines（带 base..head）；UI 场景 + capture 目检；`git diff --check` 后 commit 推 fork。

## 顺序
F1 → F2 → F3 → F4 → F5 → F6（F4-F6 独立可穿插）。

## 审查遗留风险
- baseline/manifest 重生成是 F1 最易错点，改完 struct 立即跑 schema 新鲜度测试
- gateway_http.go 仅 32 行余量、provider-editor.tsx 冻结——新代码一律新文件
- 注册是首个公开写接口：默认关闭 + 邀请码 + 限速 + 密码强度四重防护