# DHP v2 企业版部署验收 / 本地烟雾测试操作单

这份文档**同时适用于两个场景**——同一套步骤、同一套服务、同一种判断标准：

- **本地：** 开发者在 Mac/Windows 上模拟一次企业部署，验证整套链路通畅。
- **企业上线后：** 内网 ops 部署完 dhp-registry + 企业版 Halo 客户端，按本单走一遍确认部署成功。

两者唯一的区别是 `REGISTRY_URL` 和 `TOKEN`——其余完全一致。

---

## 0. 环境准备

| 角色 | 本地 | 企业 |
|---|---|---|
| Registry URL | `http://127.0.0.1:18181` | 例 `http://10.107.118.2:18081` |
| Registry Token | `enterprise-local-test` | 部署时注入的真 token |
| Registry config | `server/deploy/config.local-test.yaml` | ops 维护的 `/etc/dhp/config.yaml` |
| 客户端 product.json | 从 `halo-local/product.webank.local.json` 复制 | CI 在出包时 `cp product.webank.json → product.json` |

下文用 `$REG` 代指 Registry URL、`$TOK` 代指 Token。

---

## 1. 启 Registry

**终端 1**（保持开着）：

```bash
cd /Volumes/s790/Halo/digital-human-protocol/server

# 编一次二进制（已编过可跳）
make build              # 或: go build -o bin/dhp-registry ./cmd/registry

# 启
./bin/dhp-registry --config=deploy/config.local-test.yaml
```

**期望日志**：

```
config: deploy/config.local-test.yaml not found — using built-in defaults
   ← 不会出现这条；config.local-test.yaml 是存在的
[index-rebuild] complete: rebuilt=0 skipped=0 (digital-humans=0 skills=0 mcps=0)
dhp-registry listening on :18181 (storage=local, auth=true)
```

**期望 healthz**（开新终端验）：

```bash
curl http://127.0.0.1:18181/healthz
# {"status":"ok","time":"..."}
```

**失败排查**：

- `mkdir /tmp/dhp-local-enterprise/data: permission denied` → 你可能在 `/tmp` 没写权限，改 `config.local-test.yaml` 的 `storage.config.path`
- `listen tcp :18181: bind: address already in use` → `lsof -iTCP:18181 -sTCP:LISTEN` 找占用进程，kill 掉

---

## 2. 种几条数据进 Registry

**终端 2**：

```bash
cd /Volumes/s790/Halo/digital-human-protocol
bash server/scripts/seed-registry.sh http://127.0.0.1:18181 enterprise-local-test
```

**期望**：

```
▶ Sanity: registry healthy?
  ✓ http://127.0.0.1:18181/healthz returns 200
▶ Publish packages/digital-humans/hn-daily
  ✓ hn-daily → verdict=pass
▶ Publish packages/skills/openkursar/x-search
  ✓ openkursar/x-search → verdict=pass
▶ Result
  published=2  failed=0
▶ Registry now exposes
  digital-humans.json:
            "slug": "hn-daily",
  skills.json:
            "slug": "openkursar/x-search",
```

> 企业场景这一步等同于 ops 把"公司默认数字人"灌一次。可以传更多 bundle 路径：
> `bash server/scripts/seed-registry.sh $REG $TOK packages/digital-humans/{hn-daily,ai-daily-news} packages/skills/openkursar/{x-search,x-post}`

---

## 3. 起 Halo 客户端

**终端 3**：

```bash
cd /Volumes/s790/Halo/halo

# 把企业版本地测试 product.json 拷到根（npm run dev 会读这里）
cp halo-local/product.webank.local.json product.json

# 启
npm run dev
```

**第一屏期望**：登录页，看到 "WeBank" 一键登录按钮（绿色背景，⚡ 图标）作为推荐项。这证明 `product.json` 加载生效。

> 企业用户实际体验：他们装好企业版 Halo 后第一次启动看到的也是这一屏，只是 token / URL 已经被打包进二进制了。

---

## 4. 浏览 Store —— 验拉取

**操作**：登录后 → 点左侧 / 顶部的 "Store" 图标。

**期望**：

- 默认 tab "Digital Humans" 显示 1 个卡片：**Hacker News Daily**
- 切到 "Skills" tab：显示 1 个卡片：**openkursar/x-search**
- 切到 "MCP" tab：空

**期望日志（DevTools Console，Cmd+Opt+I）**：

```
[HaloAdapter] Loaded split index (digital-humans + skills + mcps): 2 apps total (XXms)
```

**期望 Registry 日志**（终端 1）：

```
GET /digital-humans.json 200 ...
GET /skills.json 200 ...
GET /mcps.json 200 ...
```

**失败排查**：

- store 一片空白 → DevTools Network 看请求是否打到 `127.0.0.1:18181`，如果打到 `10.107.118.2` 说明 product.json 没刷新（重启客户端）
- 401 → `enterprise-local-test` 没对上，对比 `product.json` 的 `publish.token` 和 `config.local-test.yaml` 的 `auth.token`

---

## 5. 安装数字人 —— 验下载 + 自动安装依赖

**操作**：点 "Hacker News Daily" 卡片 → 进详情页 → 点 "Install"。

**期望**：

- 进度条出现 → 完成
- "Apps" 页面新增一条 "Hacker News Daily"
- 如果 hn-daily 的 spec 里声明了 `requires.skills: [openkursar/x-search]`，**自动安装**会把这个 skill 也装进来

**Registry 日志**应看到至少：

```
GET /apps/hn-daily/2.0.0/spec.yaml 200 ...
```

> 这一步证明客户端能消费服务端的 split index → 拉具体 spec → 落盘 → 激活 runtime 全链路。

---

## 6. 单独安装 Skill —— 验 scoped slug

**操作**：回 Store → "Skills" tab → 点 `openkursar/x-search` → "Install"。

**期望**：

- 安装成功（如果第 5 步自动安装过，这里会显示 "Already installed"，正常）
- DevTools 里能看到客户端请求 `/apps/openkursar/x-search/1.0.0/spec.yaml`（scoped slug 路径正确）

---

## 7. 发布数字人 —— 验上行链路

**操作**：

1. 找一个已安装的数字人（最好是你刚装的 hn-daily，或自己手搓的）
2. 详情页点 "Publish" 按钮
3. 弹确认对话框 → 确认

**期望**（两种结果都可能，都算正常）：

| 结果 | 原因 | 验证手段 |
|---|---|---|
| **成功** | 是新版本或新 slug | Registry 日志 `POST /apps 200`，客户端弹成功 toast |
| **失败（verdict=warn）** | slug+version 已存在（你刚才 publish 过 hn-daily@2.0.0） | Registry 日志 `POST /apps 422`，客户端显示评审结果 |

> 想强制看成功路径：先编辑 hn-daily 的 `spec.yaml`，把 `version` 改成 `2.0.99`，再点 Publish。

---

## 8. 升级策略 dropdown —— 验持久化

**操作**：进 Apps → 选 hn-daily → 详情面板找 "Upgrade Strategy" 下拉。

**期望**：

- 默认是 "Auto"
- 可切换 "Notify" / "Manual"
- 切换后立即生效（不需要重启）；再开同一页面，记住所选值（验数据库持久化）

---

## 9. 重启 Registry —— 验索引重建

**操作**：

```bash
# 终端 1 按 Ctrl-C 杀掉 dhp-registry
^C

# 再启同一条命令
./bin/dhp-registry --config=deploy/config.local-test.yaml
```

**期望日志**：

```
[index-rebuild] complete: rebuilt=2 skipped=0 (digital-humans=1 skills=1 mcps=0)
```

**客户端验证**：回 store → "Refresh"（如果有按钮）或重启 Halo → 内容仍然是 2 条，**没丢**。

> 这一步是上线最关键的回归：服务节点重启不影响业务。

---

## 10. 错 token —— 验鉴权

**操作**：

```bash
# 编辑 product.json，临时把 publish.token 改成 "wrong-token"
# 重启 Halo (Ctrl-C 终端 3，重新 npm run dev)
```

**操作**：在 Apps 页面找已安装数字人 → 点 Publish。

**期望**：

- 客户端弹错误对话框
- Registry 日志：`POST /apps 401`

**测完别忘改回 `enterprise-local-test`**。

---

## 11. 占位符 token —— 验客户端 fail-fast

**操作**：

```bash
# 把 publish.token 改成 "REPLACE_AT_DEPLOY_TIME"
# 重启 Halo
```

**操作**：点 Publish。

**期望**：

- 客户端 **不发出任何 HTTP 请求**（Registry 日志没有 POST /apps）
- 客户端弹"placeholder token; not deployed properly"类似的错误

> 这一步证明 fail-fast 防护到位——企业 ops 万一忘了替换 token，客户端不会真的把占位符 token 发出去，避免日志泄露。

---

## 12. 收尾

```bash
# 终端 1: Ctrl-C 杀 registry
^C

# 终端 3: Ctrl-C 杀 Halo
^C

# 清磁盘
rm -rf /tmp/dhp-local-enterprise

# 还原客户端配置（如果不想 product.json 留着）
rm /Volumes/s790/Halo/halo/product.json
```

---

## 13. 通过标准

| 步骤 | 通过条件 |
|---|---|
| 1 | Registry 启动 + healthz 200 |
| 2 | 2 条 published 全成功 |
| 3 | 登录页见 WeBank provider |
| 4 | Store 两 tab 都见到 seed 内容 |
| 5 | hn-daily 安装到 Apps 页 |
| 6 | x-search scoped slug 路径正确 |
| 7 | 发布成功 **或** 收到合理的 verdict 反馈 |
| 8 | 升级策略可改可记 |
| 9 | 重启后 `rebuilt=2`，客户端仍可见 |
| 10 | 错 token → 401 |
| 11 | 占位符 token → 不发请求 |

12 步全通 = **企业部署链路已验证**，剩下只是替换 URL / Token / AI endpoint 这种纯配置改动。

---

## 14. 企业上线后跑这份单

把所有 `127.0.0.1:18181` 换成生产 URL（如 `10.107.118.2:18081`）、把 `enterprise-local-test` 换成真 token。其余步骤一字不改。跑完 12 步全通 = 部署成功。
