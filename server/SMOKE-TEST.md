# DHP v2 本地企业版烟雾测试

本文档描述如何在**单台开发机上**模拟一次完整的企业部署（如 WeBank 的 `10.107.118.2:18081`），跑通：

```
Halo 客户端  →  POST /apps    →  dhp-registry  →  storage + index
            ←  GET /skills.json
            ←  GET /apps/.../files/*
```

成功跑通后，企业实施只需要把同一份 `config.yaml`、同一份二进制、同一份 `product.<企业>.json` 配置部署到内网即可——本机已验证过的就是上线即用的路径。

---

## 一、自动化部分（Go 端，~30 秒）

`server/scripts/e2e-enterprise.sh` 已经覆盖：

| # | 步骤 | 验证 |
|---|---|---|
| 0 | 编译 `dhp-registry` 二进制 | `go build` 成功 |
| 1 | 启动 Registry on `:18181`，token auth，本地存储 | `/healthz` 200 |
| 2 | 全空注册表 | `/digital-humans.json` 和 `/skills.json` 都是 `[]` |
| 3 | 发布 `hn-daily`（real spec，带 token） | 200 + verdict |
| 4 | 发布 `openkursar/x-search`（scoped skill，3 个文件） | 200 + 接受 scoped slug |
| 5 | 索引含 `hn-daily` + `openkursar/x-search`，legacy `/index.json` 带 `deprecated` | 三份 JSON 都正确 |
| 6 | 下载 `index.js` 验 sha256 == 源文件 | 字节一致 |
| 7 | **重启 Registry → 索引仍在**（关键回归） | log 里 `[index-rebuild] complete: rebuilt=2` |
| 8 | 无 token / 错 token / 空 spec → 401/400 | 鉴权防御正确 |

跑法：

```bash
cd digital-human-protocol
bash server/scripts/e2e-enterprise.sh
# 失败会显式打出哪一步炸了；通过会打 "E2E PASSED"
```

环境变量：

| 变量 | 默认 | 作用 |
|---|---|---|
| `PORT` | `18181` | Registry 监听端口（避开 :8080 / :18081 冲突） |
| `KEEP_WORKDIR` | `0` | 设 `1` 保留 `/tmp/dhp-e2e-XXX/` 便于事后查 log + 存储 |

---

## 二、人工部分（Halo 客户端，~10 分钟）

服务端自动化覆盖了协议层。客户端这一段需要真人在 Electron 里点几下，验证：

1. 客户端能从本地 Registry 看到我们刚发布的内容
2. 安装 → 文件落到本地、可执行
3. 发布按钮 → 走 `http-registry` 路径上传成功
4. 升级策略 dropdown 可见、可改

### 2.1 准备客户端（覆盖 `product.webank.json`）

在 Halo 仓库的 `feature/app-market-20260514` 分支上，把 `product.webank.json` 临时改一下，让它指向本机 Registry：

```diff
   "registryOverrides": {
     "official": {
-      "url": "http://10.107.118.2:18081",
+      "url": "http://127.0.0.1:18181",
       "name": "WeBank Digital Human Registry",
       "publish": {
         "target": "http-registry",
-        "token": "REPLACE_AT_DEPLOY_TIME"
+        "token": "e2e-test-token-fixed"
       }
     },
```

> 注意：本地烟雾测试用固定 token（如 `e2e-test-token-fixed`）；启动 Registry 时也用同一个 token。**不要把这个本地 token 提交到 git**。

启动 Registry（覆盖 token，匹配 product.webank.json）：

```bash
# 在第一个终端：
cd digital-human-protocol/server
mkdir -p /tmp/dhp-smoke-data
cat > /tmp/dhp-smoke-config.yaml <<EOF
listen: ":18181"
storage: { type: local, config: { path: "/tmp/dhp-smoke-data" } }
auth:    { type: token, token: "e2e-test-token-fixed" }
rules:
  enabled: [schema_valid, slug_unique, dangerous_permissions, sensitive_categories]
EOF
go run ./cmd/registry --config=/tmp/dhp-smoke-config.yaml
```

启动客户端：

```bash
# 在第二个终端：
cd halo
HALO_PRODUCT=webank npm run dev    # 用 webank 配置启动开发版
```

### 2.2 手工 checklist

**第 1 步：拉取索引**

- 打开客户端 → Store 页面
- 切到 "Digital Humans" tab，应看到一个空列表（或仅有内置的）
- 切到 "Skills" tab，同样空
- 在客户端日志里搜 `[HaloAdapter]`，应看到：
  ```
  [HaloAdapter] Loaded split index (digital-humans + skills + mcps): 0 apps total (...)
  ```

**第 2 步：先在 Registry 端 publish 内容（用脚本）**

```bash
# 第三个终端，复用 e2e 脚本的 publish 步骤：
TOKEN="e2e-test-token-fixed" PORT=18181 bash server/scripts/e2e-enterprise.sh
```

跑完后客户端**不会自动刷新**——这是设计：要么等周期同步（6h），要么手动点 "Refresh Store" 按钮。

**第 3 步：客户端 → 看到新发布的内容**

- Store 页面 → "Refresh"
- "Digital Humans" tab 现在应该有 1 个 `hn-daily`
- "Skills" tab 现在应该有 1 个 `openkursar/x-search`
- 点 `hn-daily` → 详情页应能加载（spec 通过 `/apps/hn-daily/2.0.0/spec.yaml` 拉到）

**第 4 步：安装数字人**

- 点 "Install" 按钮
- 客户端应：
  1. 调 `installFromStore('hn-daily', spaceId)`
  2. 通过 adapter `fetchSpec` 拉 spec
  3. 通过 `installRequiredSkills` 自动安装依赖的 scoped skill `openkursar/x-search`（如果 spec 声明了）
  4. Apps 页面出现新安装的 hn-daily

- 在 Registry log 里应看到对应的 `GET /apps/hn-daily/2.0.0/spec.yaml` 请求

**第 5 步：发布按钮（http-registry 路径）**

- 在 Apps 页面找到任意一个已安装数字人（最好是自己创建的本地数字人，避免被 `slug_unique` 阻挡）
- 点 "Publish" 按钮 → 弹确认对话框
- 确认后客户端应：
  1. 通过 `publish.service` 读 `product.webank.json` 的 `publish.target=http-registry`
  2. 把当前 spec + 文件打成 multipart，POST 到 `http://127.0.0.1:18181/apps`，带 Bearer `e2e-test-token-fixed`
  3. 看到 200 响应 + verdict
- 在 Registry log 应看到 `POST /apps 200 ... ` 行

> 如果发布的是已存在的 `hn-daily@2.0.0`，会收到 422 + verdict=warn（`slug_unique` 命中），属正常防御。改 version 后重试。

**第 6 步：升级策略 dropdown**

- 进 Apps 详情页 → 找 "Upgrade Strategy" 下拉
- 应能在 `auto / notify / manual` 之间切换
- 切换后再开同一页面，下拉记忆所选值（验证 DB 持久化）

**第 7 步：负面验证**

- 把 `product.webank.json` 的 `token` 临时改成 `wrong-token`，重启客户端
- 点 Publish → 应收到 401 错误（客户端日志 + 服务端 log 都能看到）
- 把 `token` 改回 `REPLACE_AT_DEPLOY_TIME`（占位符），重启客户端
- 点 Publish → 客户端的 `http-registry.ts` dispatcher 会 fail-fast，**不会发出请求**，错误消息说"placeholder token not deployed"

### 2.3 重启 Registry，再走一遍

```bash
# 第一个终端：Ctrl-C 杀掉 registry，重新启动同一条命令
go run ./cmd/registry --config=/tmp/dhp-smoke-config.yaml
```

启动日志里应看到 `[index-rebuild] complete: rebuilt=2`。客户端刷新后**索引仍然有 2 个条目**——这是关键：服务端进程重启不丢索引，已发布内容仍可被拉取。

---

## 三、清理

```bash
rm -rf /tmp/dhp-smoke-data /tmp/dhp-smoke-config.yaml
git checkout -- halo-local/product.webank.json    # 还原 token / URL
```

---

## 四、对企业实施意味着什么

跑完上面的烟雾测试，等同于把企业部署需要的每一条因果链都在本地走了一遍：

| 本地表现 | 企业部署等价物 |
|---|---|
| `dhp-registry --config=...yaml` 起在 `:18181` | 同样的二进制 + 同样的 yaml 起在 WeBank 内网 `10.107.118.2:18081` |
| 客户端 `registryOverrides.official.url` 指向本地 | 企业版打包时 `product.webank.json` 已经写死内网 URL |
| 固定 token 在本地 yaml + product.json 同步 | 企业 ops 在部署流水线把同一个 token 同步进 yaml + 打包 product.json |
| AI judge 未配 → 4 条规则降级 warn | 配 WeBank AIEP 网关 → 4 条规则真正生效 |
| 索引重启重建 | 同样适用——服务节点重启不影响业务 |

**通过本烟雾测试 + 按 `server/DEPLOY.md` 走完部署 = 100% 上线就绪。**
