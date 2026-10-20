# 磁盘高级管理 交接文档

> 飞牛 fnOS 磁盘高级管理 · 交接给下一位开发者 / 维护者
> 版本 v1.0.0（首个发布版本）

> 仓库：<https://github.com/chanhuan1/Fn-SmartDisk> · 在线介绍页：<https://fn-smartdisk.pages.dev>

---

## 1. 项目是什么

磁盘高级管理（内部 appname 为 `SmartDisk`）是一个**飞牛 fnOS 原生 fpk 应用**，扩展官方仅有的基础 SMART 显示，提供更完整的磁盘健康信息。以官方 `fnpack` 打包为 `.fpk`，通过 **fnOS 统一网关** 在桌面以 iframe 窗口打开，也支持独立端口直连。

**核心能力：**

| 能力 | 说明 |
|------|------|
| 磁盘总览 | 按卷排序的卡片网格：型号/序列号/接口/主控/角色/桥接/容量/温度/寿命/健康色条与徽标，点击进详情 |
| USB 桥接识别与透传 | 识别 JMicron/ASMedia/Realtek/Sunplus/Innostor 等芯片，自动探测 SAT 透传并标注命中协议 |
| 剩余写入寿命 | NVMe `Percentage Used`；SATA SSD 按**主控厂商**识别（attr231/慧荣169/三星官方TBW估算），标注精准/估算来源 |
| 主控识别 | SSD 主控识别（群联S11/S12、慧荣SM22xx、三星、海力士、东芝/铠侠、闪迪、金士顿、建兴、威刚、浦科特、希捷等），型号+固件+属性特征推断 |
| 使用年限 | 通电时间按 7×24 折算 + 通电次数 + 小时数 |
| 容量使用量 | lsblk 分区树汇总已挂载分区已用空间与使用率 |
| 硬件信息 | WWN/EUI-64、缓存大小、NCQ 深度、外形规格、转速、逻辑/物理扇区、TRIM、固件版本、生产日期（多数盘不公开显示「未知」） |
| SMART 深度 | 自检历史（类型/结果/通电小时）、ATA 错误日志、设备统计、SCT 温度历史 |
| 磁头加载计数 | SMART 193 加载/卸载计数 + 高次数预警（HDD） |
| 链路速率/错误 | SATA 协商速率 vs 设备最大（掉速预警）、SATA PHY 链路错误计数、NVMe PCIe 链路、USB 桥接描述 |
| 分区/存储链 | 分区名/大小/文件系统/挂载点/用量；详情页存储链（盘→分区→RAID→LVM→卷） |
| btrfs 池错误 | btrfs 存储池读写/校验和/代际错误计数（数据静默损坏早期信号），计入健康评级 |
| 电源状态 | 当前电源状态（运行/空闲/休眠）+ APM 电源管理级别 |
| 休眠盘安全采集 | 首次采集先探测旋转盘状态；休眠或状态不明时显示基础信息，用户确认后逐盘读取 SMART |
| 健康评级 | 综合重映射/待处理/CRC/温度/寿命/btrfs 错误 → 优/良/警/危 + 中文风险提示 |
| 中文化 | SMART 属性（含厂商专属表）、自检、设备统计、状态全部中文为主、英文原名做次级标注；重要属性带 `*` |
| 电源控制 | 深度下线（umount -l+offline 内核隔离，飞牛无法唤醒）、唤醒（running+rescan+补挂载）；UI 仅暴露这两个动作 |
| 实时步骤日志 | 深度下线/唤醒通过 SSE 流式推送每个步骤（sync→卸载→停转→offline / running→起旋→重扫→补挂载），确认弹窗内滚动显示 |
| 操作日志 | 电源事件流水（汇总 + 逐步骤 + 补挂载，成功失败与详情），电源页面板展示，已下线时长显示 |
| 下线记录 | 持久化到 `${TRIM_PKGVAR}/hibernate.json`（重启不丢），以**序列号**为 key（盘符漂移也能唤醒）；孤儿 offline 盘（内核 offline 但无记录）采集时自动识别并显示唤醒 |
| 命令行工具 | `smartdisk-cli` 输出 JSON |

**关键约束：** SMART 数据采集只读；休眠旋转盘默认不读取 SMART，用户确认后才执行可能唤醒磁盘的读取。**电源控制是仅有的写操作**（`hdparm -y`、写 `/sys/block/sdX/device/state`），且有严格状态机与系统盘/阵列保护，见第 6.5 节。

---

## 2. 交付物与现状

| 项 | 状态 |
|----|------|
| 安装包 | `SmartDisk/SmartDisk.fpk`（约 3.2 MB，x86-64，版本 1.0.0），可手动安装 |
| 统一网关 | **已接入**（iframe 桌面窗口走 `/app/SmartDisk` → Unix Socket） |
| 电源控制 | **已实现**（深度下线/唤醒 + 确认弹窗 + 阵列/系统盘保护 + SSE 实时步骤日志 + 操作日志面板），UI 截图验证通过 |
| 单元/集成测试 | 全部通过（`smart`、`collector` 含 mock 端到端） |
| UI | 卡片式列表 + 详细详情页，截图验证通过 |
| 数据面 | A(自检/错误/设备统计) + D(分区) + E(链路) + 容量使用 + 中文化 全部落地 |
| 真机联调 | **部分** —— iframe 网关已修通；解析器仍需真实盘数据进一步校准 |
| 自带 smartctl/nvme 二进制 | **机制已就位，二进制未放** —— 见第 7 节 |

**环境提醒（重要）：** 开发机 macOS 27 (Tahoe) 与 Go 1.22 有链接兼容 bug（引入 `net` 包时生成损坏二进制，缺 `LC_UUID`）。**必须用 Go 1.24+**。本机曾用 `/private/tmp/gosdk124`（1.24.9），属临时目录，重启会被清理，请自备 Go 1.24+ 并设 `GO` 环境变量。

### 1.0.0 变更（首个发布版本）

面向用户的应用名称为「磁盘高级管理」，安装包版本为 `1.0.0`。内部 `appname`、`desktop_applaunchname` 和 `/app/SmartDisk` 网关路径保持原值。

版本内容整合了此前开发过程中的全部能力：全介质磁盘健康检测、SSD 剩余寿命与主控识别、完整中文 SMART、链路与 btrfs 错误、电源控制（深度下线/唤醒 + 独立的 `confirmRaid` 与 `confirmBusyIO` 确认语义）、界面体验优化、休眠盘安全采集与磁盘备注。该语义贯通本地模式、分离 IPC、HTTP/SSE 与前端确认弹窗，并有回归测试覆盖。

**新增组件**（`web/src/components/`）：`DiskTypeIcon.vue`（盘类型图标）、`PowerBadge.vue`（运行/休眠/已下线徽标 + 脉冲点）、`SkeletonCards.vue`（加载骨架屏，kind: overview/detail/power）、`CapacityDonut.vue`（容量环形图）、`TempRangeBar.vue`（温度区间条）。删除死代码 `TopologyTree.vue`。

**磁盘总览**：总容量改环形图 + 已用量；卡片加盘类型图标、电源徽标、用量迷你条；温度按盘类型分级配色（NVMe 70/80、SSD 60/70、HDD 50/60 阈值，见 `api.ts` 的 `tempThresholds/tempColorOf`）；采集时间「X 前 + 绝对时刻」双显。

**详情页**：指标卡按「使用强度 / 寿命与磨损」分组；SMART 属性支持关键词筛选 + 「仅看异常」开关，属性名悬浮中文释义（`api.ts` 的 `ATTR_DESC`/`attrDesc`，约 60 条）；温度区间条（历史最低/当前/最高）；分区用量迷你条；健康评估与说明按 正常/提示/警告/危险 四级配色（`note-ok/info/warn/crit` + `.n-ico` 图标）。

**通用**：加载骨架屏替换转圈；网络/HTTP 错误友好化（`netErrorText/httpErrorText`）；全局 `error`/`unhandledrejection` 兜底提示；Tab 栏 sticky + 毛玻璃；列表行悬停高亮；自定义滚动条；480px/360px 窄屏（手机）自适应。

**修复**：NVMe 盘「接口 NVMe + 类型 NVMe」标签重复——`Overview.vue` 的 `mediaTag()` 在接口为 NVMe 时把介质标签统一显示为 SSD；`icons.ts` 函数式图标 prop 对象解包（修复 `{size}` 未取值的图标渲染异常）。

安全采集优化（已并入 v1.0.0）：
- 冷缓存采集先用 `smartctl -n standby` 探测旋转盘；休眠或无法确认状态的盘设为 `smartDeferred`，不会执行完整 SMART 读取。
- 概览和详情提供逐盘确认读取；跳过后保留基础信息，`smartdisk-cli scan` 是显式全盘采集入口。
- collector 以隔离快照发布缓存，读请求不等待磁盘扫描；逐盘采集最多并发 2 块。电源操作状态更新不会被并发刷新覆盖。
- 拓扑查询复用缓存；全盘刷新只接受 POST；collector/web 模式通过逐行 JSON IPC 转发电源操作 SSE 步骤。
- 前端严格类型检查成为构建门槛，打包时清理前端残留资源与旧 `smartctl`/`nvme`，缓存数据超过 24 小时会在界面突出显示。

---

## 3. 目录结构

```
飞牛SmartDisk/
├─ SmartDisk/                  fpk 应用骨架（fnpack create 生成后改造）
│  ├─ manifest                 应用元数据（appname/version/端口/x86/os_min_version）
│  ├─ cmd/                     生命周期脚本（main=启动控制, install_callback 校验）
│  ├─ config/
│  │  ├─ privilege             run-as=root（读设备必需）
│  │  └─ resource              usr-local-linker 暴露 bin/smartdisk-cli
│  ├─ app/
│  │  ├─ server/smartdisk      ★ 编译产物：Go 单二进制（linux/amd64 静态）
│  │  ├─ www/                  ★ 前端构建产物（同时嵌入二进制）
│  │  ├─ bin/smartdisk-cli     CLI wrapper（调主二进制 cli 子命令）
│  │  └─ ui/config             fnOS 桌面入口（统一网关 iframe /app/SmartDisk）
│  ├─ ICON.PNG / ICON_256.PNG  应用图标（64/256）
│  └─ SmartDisk.fpk            ★ 最终安装包
│
├─ server/                     Go 后端源码
│  ├─ main.go                  入口（all/collector/web 三模式 + cli 子命令 + 网关监听）
│  ├─ go.mod
│  ├─ internal/
│  │  ├─ config/               环境变量配置（含网关 socket/prefix）
│  │  ├─ model/                磁盘/SMART/分区/链路/自检/NVMe 数据模型
│  │  ├─ smart/                smartctl JSON 解析、桥接库、健康评级、生产日期
│  │  │  ├─ attrnames.go       SMART 属性 ID → 中文名 + 重要性（通用表）
│  │  │  ├─ vendor.go          主控识别 + 厂商属性名表 + 厂商寿命识别 + 三星TBW表
│  │  │  ├─ translate.go       自检类型/状态、设备统计 → 中文翻译
│  │  │  ├─ identifiers.go     WWN/EUI-64 生成、SATA PHY 事件中文
│  │  │  └─ health.go          健康评级（含 btrfs 错误）
│  │  ├─ collector/            设备枚举(lsblk)、采集编排、透传探测、链路、拓扑、btrfs、IPC
│  │  │  ├─ power.go           电源控制（Sleep/Offline/Wake + 能力位 + 状态机）
│  │  │  ├─ hibernate.go       下线盘持久化映射（UUID/挂载点，唤醒时恢复）
│  │  │  └─ powerlog.go        电源操作日志（环形 200 条，power-events.json）
│  │  └─ httpapi/              REST API + 静态资源 + SPA fallback + 网关前缀
│  ├─ web/
│  │  ├─ embed.go              go:embed 前端
│  │  └─ dist/                 前端构建输出（构建时生成）
│  └─ mock/                    演示用采集样本（无真机时跑 UI/管线）
│
├─ web/                        前端源码（Vue3 + Vite + TS）
│  └─ src/
│     ├─ App.vue               hash 路由壳（#/ 列表, #/disk/:id 详情）
│     ├─ api.ts                API client + 类型 + 格式化工具（自动识别 /app/* 前缀）
│     ├─ views/Overview.vue    卡片式磁盘列表页（含电源徽标 + 快捷休眠按钮）
│     ├─ views/Detail.vue      磁盘详情页（指标卡/健康评估/各数据区块 + 电源操作）
│     ├─ views/Power.vue       电源控制页（全列表 + 状态徽标 + 已下线时长 + 操作日志面板）
│     ├─ components/           HealthBadge、PowerConfirm（电源确认弹窗）等
│     ├─ icons.ts              内联 SVG 图标（无依赖）
│     └─ style.css             设计系统（深色运维控制台，含全部 UI token）
│
├─ scripts/
│  ├─ build.sh                 一键构建：前端→交叉编译→打包 fpk
│  └─ dev.sh                   本地 mock 模式起后端
├─ tools/
│  ├─ icon.svg                 图标源文件
│  ├─ shot.mjs / shot_scroll.mjs  无头 Chrome 截图脚本（CDP，验证 UI 用）
│  ├─ set-secrets.mjs          一次性写入 Cloudflare Pages 部署所需 GitHub Secrets
│  └─ bin/                     （空）放 linux/x86_64 静态 smartctl、nvme
├─ site/                       Cloudflare Pages 在线介绍页（纯静态单页）
├─ .github/workflows/deploy.yml  push master 自动部署 Pages
├─ wrangler.jsonc              Cloudflare Pages 项目配置（fn-smartdisk）
├─ README.md                   项目说明
└─ HANDOFF.md                  本文档
```

---

## 4. 架构与数据流

```
┌─ fnOS 桌面 iframe (/app/SmartDisk) ─┐   ┌─ 浏览器直连 (IP:8315) ─┐
│   Web 面板 (Vue3, hash 路由)         │   │   同一构建，自动识别前缀  │
└─────────┬───────────────────────────┘   └──────────┬──────────────┘
          │ fnOS 网关校验登录→转发 Unix Socket        │ TCP
┌─────────▼───────────────────────────────────────────▼──────────────┐
│  smartdisk (Go 单二进制)                                            │
│  ├─ httpapi   REST(/api/*) + 静态资源 + SPA fallback + 网关前缀     │
│  ├─ collector 枚举 + 采集编排                                        │
│  │   ├─ lsblk -J -O -b        枚举 + 分区树(用量/文件系统)           │
│  │   ├─ sysfs / lsusb         USB VID:PID、NVMe PCIe 链路           │
│  │   └─ smartctl -a -j -d X   SMART(属性/自检/错误/统计/链路)        │
│  ├─ smart     解析 + 健康评级 + 中文化                               │
│  └─ model     数据结构                                               │
└─────────────────────────────────────────────────────────────────────┘
```

### 采集逻辑（`collector/collector.go`）

1. **枚举**：`lsblk -J -O -b`（不带 `-d`，保留分区树）取 `type=disk`，过滤 `loop/ram/dm-/md/sr/zram`。分区树用于容量用量汇总（D）与分区明细。
2. **接口分类**：`tran` 字段 → SATA / NVMe / USB / SCSI。
3. **冷缓存安全预检**：旋转盘先执行 `smartctl -n standby`；仅在明确报告 active/idle 时继续读取 SMART。休眠或状态无法确认时标记 `smartDeferred=true`，跳过 SMART 与 btrfs 盘级统计。
4. **分路径采集 SMART**：
   - **NVMe**：`smartctl -a -j -d nvme <dev>`（health log + PCIe 链路从 sysfs 读）。
   - **USB 桥接**：sysfs 读 `idVendor/idProduct` → 查桥接库得芯片名与首选透传协议 → 按 PassthroughOrder 依次尝试 `-d sat,usbjmicron,usbsunplus,...`，命中即停并记录。
   - **SATA/SCSI**：先试 `-d sat`，失败再 `auto`。
   - **确认读取**：用户确认后对 `smartDeferred` 磁盘逐盘调用 `POST /api/disks/{id}/refresh`；CLI 的 `scan` 执行完整全盘读取。
5. **解析 + 丰富**（`smart/`）：基础信息 + 寿命 + 自检/错误/设备统计(A) + 扇区/转速/外形/TRIM + SATA 链路(E) + 中文化 → `Grade()` 评级 → `InferManufactureDate()`。

### 运行模式（`main.go`，由 `SMARTDISK_MODE` / `SMARTDISK_GATEWAY` 控制）

| 模式 | 说明 |
|------|------|
| `all`（默认） | 采集 + Web 面板同进程。**cmd/main 当前用此模式** |
| `collector` | 仅 root 采集守护，Unix socket 提供数据 |
| `web` | 仅 Web 面板，经 Unix socket 向 collector 取数（最小权限拆分，已预留） |
| `cli` | `smartdisk cli list` / `get <id>` 安全读取；`scan` 明确触发完整采集，输出 JSON 即退 |

**网关开关**：`SMARTDISK_GATEWAY=1` 时，HTTP 服务监听 **Unix Socket**（默认 `${TRIM_APPDEST}/app.sock`）且所有路由挂在 `SMARTDISK_GATEWAY_PREFIX`（`/app/SmartDisk`）下；否则监听 TCP 端口、无前缀。前端 `api.ts` 运行时从 `location.pathname` 自动检测 `/app/*` 前缀，同一份构建兼容两种访问。

> **实测结论**：fnOS 网关对 prefix 大小写不敏感（`/app/SmartDisk` 与 `/app/smartdisk` 都会路由到同一 socket），但**网关注册在应用安装/重启时生效**——改入口配置后必须重装或重启应用才会建立路由。网关转发前会校验 NAS 登录态（未登录的 API 请求返回 `invalid token`），浏览器已登录则透明通过。为稳妥，路由同时注册了带前缀与剥前缀两种形式，兼容网关透传或 strip 的行为。

**权限说明**：读 SMART 需 `/dev/sdX`、`/dev/nvmeX` 原始访问，故按官方「访问包用户无法处理的设备」场景以 root 运行（`config/privilege: run-as=root`）。采集与面板的最小权限拆分代码已实现（`collector/ipc.go`），如需启用，把 cmd/main 改成分别起 collector（root）+ web（runuser 降权）即可。

---

## 4.5 电源控制（休眠 / 深度下线 / 唤醒）

第一版之后的**唯一写操作**功能，把社区「内核级硬盘休眠」方案软件化（参考飞牛论坛离线休眠实践）。三个动作，全部经 `collector/power.go` 的状态机与保护。

**适用介质：仅 SATA 机械硬盘（HDD）**。固态硬盘（SATA SSD / NVMe）无机械结构、无需停转，不提供休眠/下线入口（按钮禁用并提示「固态硬盘不需要休眠」）；USB 桥接盘因透传不可靠同样禁用。面板顶部与确认弹窗均有此提示。

| 动作 | 命令序列 | 效果 |
|------|----------|------|
| **深度下线** `Offline` | `sync` → `umount -l <卷>`（懒卸载）→ `hdparm -y` → `echo offline > /sys/block/sdX/device/state` | 内核级隔离，**飞牛后台 SMART/文件管理器都无法再唤醒**，只能本面板手动唤醒 |
| **唤醒** `Wake` | `echo running > state` → `echo 1 > rescan` → 轮询最多 15s 等设备节点 → 按持久化映射补 `mount UUID=<uuid> <挂载点>` → 强制重采 | 恢复上线；md 阵列上的 btrfs 通常由内核重组自动回挂，无需手动 mount |

> 说明：`Sleep`（hdparm -y 普通休眠）后端 API 仍保留，但 **UI 已不暴露**——普通休眠会被 fnOS 随时唤醒，与「永不唤醒」目标相悖，故面板只提供 深度下线 / 唤醒 两个动作。

**保护机制（`computePowerCaps`）：**

- **系统盘**（挂载 `/`、`/boot`）三个操作一律拒绝。
- **仅 SATA 机械盘**可操作；NVMe / SSD / USB 桥接按钮禁用并给出原因（`NoSleepReason`/`NoOfflineReason`）。
- **md RAID 成员盘**深度下线需前端勾选「我已了解阵列降级风险」→ 请求体带 `confirmRaid:true`，否则 API 返回 409。磁盘正在读写时另需独立的 `confirmBusyIO:true`，二者不能互相替代。
- 磁盘 id 全量校验（`^[a-z0-9]+$`），命令经统一 `runner` 执行。
- **挂载点路径校验**（`validateMountPoint`）：umount/mount 以 root 运行，拒绝非绝对路径、`..`、空格/分号/管道/`$` 等字符，防止 hibernate 映射被篡改后注入命令。
- **懒卸载（`umount -l`）**：fnOS 的 `trim_file_monit` 对每个卷都保持只读目录句柄，严格 `umount` 必返回 EBUSY。先 `sync` 刷盘后用懒卸载摘掉挂载点，内核在句柄释放后完成清理——这是社区冷备方案的通行做法，对只读监控句柄无数据风险。
- **电源操作串行化**（`wakeMu`）：Sleep/Offline/Wake 全局互斥，防并发把状态机打乱。
- **降级阵列告警**：唤醒 RAID 成员盘后若阵列未重组（`VolumePaths` 为空），结果与日志明确标注「阵列未重组，存储空间暂不可用」，不伪装成功。
- **降级阵列告警**：只有「下线时原本有挂载卷、唤醒后卷没回来」才提示「存储空间暂未回挂，请检查」；**下线时本来就没卷的盘**（如无卷冷备盘）唤醒后没卷是正常的，不误报失败。
- **读写活动检测 + 二次确认**：下线前读两次 `/proc/diskstats`（`diskIOActive`，间隔 300ms），若该盘读+写扇区计数在涨，返回 `ErrDiskBusy`(409) 拒绝下线；前端弹「正在读写，强制下线可能丢数据」强确认框，勾选后才放行。探测失败时安全降级为不拦。
- **补挂载失败保留下线标记**：唤醒时若有挂载点恢复失败，磁盘在面板仍显示「已下线」可重试唤醒，不会静默从列表消失。
- **唤醒重扫顺序**：`running` → 等待设备节点出现 → `rescan`（先等设备是阵列成员，再重扫读分区表，利于内核重组阵列）。
- **重启状态校正**：采集时读每盘 `/sys/block/sdX/device/state`。记录标 offline 但内核已 running（飞牛重启会唤醒所有盘）→ 自动清掉陈旧 offline 标记，盘显示为在线（可重新下线）；内核确实还 offline → 保持「已下线」+「唤醒」。避免重启后卡在「已下线」假象。
- **唤醒不打扰其他盘**：唤醒只重采这一块盘（`refreshSingleDisk`），不全量 Refresh——否则 `smartctl -a` 会把其他休眠盘也唤醒。

**持久化（`collector/hibernate.go`）：** 每次成功采集后把每盘的 `{id, wwn, serial, 分区UUID, 挂载点, 阵列标记}` 写入 `${TRIM_PKGVAR}/hibernate.json`（`SMARTDISK_DATA_DIR` 覆盖）。盘下线后从 lsblk 消失，唤醒靠这份映射补挂载；`Noauto` 标记的（md 阵列上的文件系统）唤醒时不手动 mount，等内核重组。

**API：** `POST /api/disks/refresh` 执行全盘刷新；仅允许 POST。`POST /api/disks/{id}/refresh` **只刷新单盘**（用户确认读取暂缓 SMART 时使用，不打扰其他盘）。`GET /api/topology` 复用安全采集缓存，不主动重读 SMART。`Disk` / `Summary` 新增 `smartDeferred`。电源接口为 `POST /api/disks/{id}/sleep|offline|wake`（`offline` 接受独立的 `{"confirmRaid":true}` 与 `{"confirmBusyIO":true}`；前者确认阵列降级风险，后者确认正在读写时强制下线）；SSE 模式下 offline/wake 逐步推送日志。分离模式通过逐行 JSON IPC 保留这些步骤。错误带语义化 HTTP 码（403 系统盘/不支持、409 需阵列/IO 确认、400 未下线/不存在）。`GET /api/power/log?limit=N` 返回操作日志（newest first）。

**操作日志（`collector/powerlog.go`）：** 每次操作记录一条**汇总** `PowerEvent{time, diskId, model, action, ok, detail}`（逐步的实时步骤只经 SSE 推到确认弹窗，**不写入持久化日志**——面板一次操作一行，保持简洁）。环形保留最近 200 条，持久化到 `${TRIM_PKGVAR}/power-events.json`（`SMARTDISK_DATA_DIR` 覆盖）。已下线的盘在状态徽标下额外显示「已下线 x 时间」（`Disk.OfflineAt`）。

**UI：** 顶栏「电源控制」Tab（全列表 + 状态徽标 + 操作按钮，禁用态 tooltip 显示原因；顶部横幅提示「固态硬盘不能休眠，仅机械硬盘可休眠」）；总览卡片右上角实时电源状态芯片（运行中/休眠/已下线，带呼吸点）；详情页头部电源状态徽标 + 深度下线/唤醒按钮 + 单盘刷新按钮；唤醒按钮为亮薄荷绿（明显可点）；`components/PowerConfirm.vue` 统一确认弹窗（下线强确认列卷+风险+阵列勾选 / IO 确认 / 唤醒起旋等待 + 实时步骤日志 + 挂载硬盘提示）。「刷新」入口语义：总览顶部=全量重采（会唤醒休眠盘），详情页=只刷该盘。

---

## 5. 本地开发

**依赖：** Go 1.24+、Node 18+、fnpack（打包用）。

### 5.1 前端

```
cd web
npm install
npm run typecheck  // 严格类型检查
npm run build      // 产物输出到 server/web/dist（被 Go embed）
npm run dev        // 热更开发（/api 代理到 8315）
```

### 5.2 后端（mock 模式，无需真机）

```
cd server
SMARTDISK_MOCK_DIR=./mock go run .
// 打开 http://localhost:8315
```

mock 模式读 `server/mock/` 下的样本文件而非真实命令，文件名由 `mockKey()` 从命令行生成（规则见 `collector/runner.go`，集成测试 `integration_test.go` 有断言）。**新增样本时文件名必须与 mockKey 生成的一致**，手写的容易错——建议先用 `TestMockKeyStable` 或调试打印拿到真实 key 再命名（NVMe sysfs 链路样本就是 `cat__sys_class_nvme_...`）。

### 5.3 测试

```
cd server
go test ./...        // 单元 + 集成
go test -race ./...  // 缓存快照与 IPC 并发
go vet ./...
gofmt -l .           // 应为空
```

### 5.4 常用环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `SMARTDISK_MODE` | `all` | all / collector / web |
| `SMARTDISK_PORT` | `8315` | TCP 端口（非网关模式） |
| `SMARTDISK_GATEWAY` | 空 | `1`/`true` 启用统一网关（Unix socket） |
| `SMARTDISK_GATEWAY_SOCKET` | `${TRIM_APPDEST}/app.sock` | 网关 socket 路径 |
| `SMARTDISK_GATEWAY_PREFIX` | `/app/SmartDisk` | 网关路由前缀 |
| `SMARTDISK_MOCK_DIR` | 空 | 非空即 mock 模式，指向样本目录 |
| `SMARTDISK_WEB_ROOT` | `<appdest>/www` | 前端目录（embed 为空时回退） |
| `SMARTDISK_SMARTCTL` / `SMARTDISK_NVME` | 自动探测 | 工具路径（优先 app/bin） |
| `SMARTDISK_DATA_DIR` | `${TRIM_PKGVAR}` | 运行时状态目录（`hibernate.json` / `power-events.json`） |
| `SMARTDISK_HDPARM` | 自动探测 | hdparm 路径（默认 `/usr/sbin/hdparm`） |

---

## 6. 构建与打包

```
GO=/path/to/go scripts/build.sh
```

产物为 `SmartDisk/SmartDisk.fpk`。`build.sh` 先严格检查前端类型与 manifest/package 版本，再构建并打包：

0. **读取版本号**：从 `SmartDisk/manifest` 的 `version=` 提取（唯一事实源），失败即中止
1. `web/` 下 `npm run typecheck && npm run build` → `server/web/dist`
2. 交叉编译 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`，`-extldflags=-static` 静态链接，并 `-X main.Version=$VERSION` 注入版本 → `SmartDisk/app/server/smartdisk`
3. 清空并同步前端到 `SmartDisk/app/www/`
4. 删除暂存目录里的旧 `smartctl`/`nvme` 后，只复制 `tools/bin/` 中实际存在的版本，保留 `smartdisk-cli`，最后执行 `fnpack build`

### 版本号管理（单一事实源）

版本号**只在 `SmartDisk/manifest` 的 `version=` 一处维护**。发新版时改这一处，然后：

- 同步 `web/package.json` 的 `version`（保持一致的约定）
- 同步 `HANDOFF.md` 标题行的「版本 vX.Y.Z」

运行时链路：`build.sh` 读 manifest → `-X main.Version` 注入二进制 → `main.go` 写入 `cfg.Version` → `/api/health` 返回 `version` → 前端 App.vue 在 Tab 栏右侧实时显示 `vX.Y.Z`。**UI 显示的版本号永远等于装进设备的 manifest 版本**，不依赖手动改前端。

**打包前检查清单：**

- `find SmartDisk -name ".DS_Store" -delete`（mac 会产生，勿带入包）
- 二进制确认为 x86-64：`file SmartDisk/app/server/smartdisk`
- `config/resource` 声明的文件必须存在（usr-local-linker 指向 `bin/smartdisk-cli`）
- 版本号：`SmartDisk/manifest` 的 `version` 与 `changelog`

**安装到 fnOS：** 应用中心 → 手动安装 → 上传 `.fpk`；或用 `appcenter-cli`。

---

## 7. 待办：自带 smartctl / nvme 静态二进制

**现状**：应用运行时优先用 `SmartDisk/app/bin/` 下的 smartctl/nvme，缺失则回退系统命令（`config.Load` 的探测逻辑）。fnOS 基于 Debian 通常自带 smartmontools，所以**当前回退方案能工作**；但要做到完全不依赖系统环境，需自带二进制。

**要做的事**：把 **linux x86_64 静态编译**的这两个文件放入 `tools/bin/`：

- `smartctl`（smartmontools，建议 7.x）
- `nvme`（nvme-cli）

然后 `scripts/build.sh` 会自动打进 `app/bin/`。

**获取途径（任选）**：在 Debian/Alpine x86_64 容器里安装后取二进制（注意 glibc 动态库依赖，最好用 musl 静态构建或 staticx 处理）；或从源码静态编译（`./configure LDFLAGS=-static`）。

因开发机是 macOS arm64，无法本地产出 Linux x86_64 静态二进制，故这一步留到 Linux 环境做。可借助 Docker 取动态版本再处理依赖：

```
docker run --rm -v "$PWD/tools/bin:/out" debian:bookworm \
  bash -c 'apt-get update && apt-get install -y smartmontools nvme-cli && \
           cp /usr/sbin/smartctl /usr/sbin/nvme /out/'
```

（产出为动态链接，需另处理依赖；静态链接更稳。）

---

## 8. 真机联调指南（下一步重点）

iframe 网关已修通，但**解析器面对的是千差万别的真实盘**，仍需真机校准。

1. **安装**：把 `.fpk` 装到 fnOS 测试机，桌面 iframe 窗口应能正常打开（走 `/app/SmartDisk`）。
2. **看日志**：`cmd/main` 写到 `${TRIM_PKGVAR}/smartdisk.log`（通常在 `/vol*/@appdata/SmartDisk/`）。
3. **CLI 直采**：SSH 进 fnOS，跑

   ```
   smartdisk-cli list          // 安全读取；休眠旋转盘标记 smartDeferred
   smartdisk-cli get sda       // 单盘安全详情
   smartdisk-cli scan          // 明确执行完整采集，可能唤醒休眠盘
   ```

4. **对照原始输出**：对有问题的盘，取原始数据发回来校准：

   ```
   lsblk -J -O -b
   smartctl --scan
   smartctl -a -j /dev/sdX            // 每块盘
   smartctl -a -j -d sat /dev/sdX     // USB 盘试透传
   nvme list
   nvme smart-log /dev/nvme0
   lsusb
   cat /sys/class/nvme/nvme0/device/current_link_speed   // NVMe 链路
   ```

5. **常见要校准的点**：
   - 某 USB 桥接芯片透传协议不在库里 → 补 `smart/bridge.go` 的 `usbBridgeDB`；
   - 某 SATA SSD 寿命属性 ID 不在已识别集合 → 补 `smart/smartctl.go` 的 `deriveATALife`；
   - 某属性 ID 显示「-」（无中文名）→ 补 `smart/attrnames.go`；
   - 自检/设备统计英文没翻译 → 补 `smart/translate.go`；
   - 生产日期规则 → 补 `smart/mfgdate.go`（**宁可显示未知，不要猜错**）；
   - 温度/健康阈值按介质类型在 `smart/health.go` 的 `tempThresholds`。
6. **不同盘差异**：老盘可能没有 Device Statistics、USB 桥接盘可能读不到自检日志，这些区块会自然不显示，属正常。

---

## 9. 二次开发指引

### 加一个 SMART 扩展指标

1. `model/model.go` 的 `Disk` 加字段（带 json tag）。
2. `smart/smartctl.go` 的 `ParseSmartctl` 里从 `smartctlJSON` 提取填入（如需先在结构体加对应 JSON 字段）。
3. `web/src/api.ts` 的 `Disk` 接口加对应字段，`Detail.vue` 里展示。
4. `server/mock/` 加样本 + `smart/smart_test.go` 加断言。

### 加一个 USB 桥接芯片

改 `smart/bridge.go` 的 `usbBridgeDB`：`"vvvv:pppp": {Chip: "厂商 型号", Passthrough: "首选协议"}`。配 `smart_test.go` 的 `TestBridgeDB` 验证。

### 加属性中文名 / 翻译

- SMART 属性 ID → 中文：改 `smart/attrnames.go` 的 `attrMetaByID`（含是否重要 `Important`）。
- 自检类型/状态、设备统计 → 中文：改 `smart/translate.go`。

### 加主控识别 / 厂商寿命规则

- 新增品牌/主控识别：改 `smart/vendor.go` 的 `vendorKeys`（型号 token）与 `ControllerLabel()`（显示标签）；贴牌→主控映射加 `brandCtrl`。
- 新增厂商属性名：改 `vendor.go` 的 `vendorAttrNames`。
- 新增三星 TBW 数据：改 `vendor.go` 的 `samsungTBW`（只收录有官方 datasheet 依据的型号，**不臆造**）。
- 厂商寿命分派逻辑在 `deriveVendorLife()`（优先级 attr231 → 慧荣169 → 三星TBW估算）。

### 调整健康评级规则

改 `smart/health.go` 的 `Grade()`：各指标加分（score 越高越差），阈值映射到 优/良/警/危。`TestHealth*` 系列测试对照。

### 改电源控制 / 单盘采集

- 电源操作（下线/唤醒/能力判定/IO 检测）：`collector/power.go`。下线= `offline()`、唤醒= `wake()`、能力位= `computePowerCaps()`、IO 检测= `diskIOActive()`。
- 单盘采集（不打扰其他盘）：`collector/power.go` 的 `refreshSingleDisk()`，由详情页刷新（`POST /api/disks/{id}/refresh` → `RefreshDisk`）和唤醒流程复用。
- 下线记录：`collector/hibernate.go`（key=序列号）；日志：`collector/powerlog.go`（只写汇总）。

### 前端改动

前端是标准 Vite + Vue3 SFC。改完 `npm run build`，再重新交叉编译后端（embed 会带上新 dist）。本地调 UI 用 mock 模式 + `npm run dev`。

**UI 验证**：用 `tools/shot.mjs` / `tools/shot_scroll.mjs`（无头 Chrome CDP 截图）对着本地 mock 服务截图检查效果，参考第 5.2 节起服务方式。

---

## 10. 已知限制与路线图

**当前限制：**

- 仅 x86_64（manifest `platform=x86`）。
- 采集只读；电源控制是仅有写操作（见 4.5），第一版不含 SMART 自检触发、定时巡检、告警。
- 深度下线后**卸载应用不会自动唤醒**盘——重启 fnOS 或在本面板手动唤醒恢复（文档需告知用户）。
- 生产日期大多显示「未知」（厂商不公开，符合预期）。
- 温度/寿命无历史曲线（当前为即时值）。
- 自带 smartctl/nvme 静态二进制未打包（见第 7 节），现依赖系统命令回退。
- 「已使用容量」是文件系统层面（已挂载分区用量），未分区/未挂载的盘不显示已用百分比，属正常。

**路线图：**

- SMART 自检（short/long）触发与进度展示（变可写，需谨慎）
- 定时巡检 + 异常告警（fnOS 通知中心 / webhook）
- 温度与寿命历史曲线（需在 var 目录落库 + 后台定时任务 + 前端图表）
- ~~电源控制（休眠/深度下线/唤醒）~~（已实现，见 4.5）
- ~~主控识别~~（已实现）· ~~厂商/主控级寿命识别 + 三星TBW估算~~（已实现）· ~~btrfs 池错误计数~~（已实现）· ~~电源状态/APM~~（已实现）
- 自带静态 smartctl/nvme（去系统依赖）
- ARM 架构支持（交叉编译 + 多架构二进制）
- 主题切换（浅色 / 更强品牌色）

---

## 11. 排错速查

| 现象 | 排查 |
|------|------|
| iframe 窗口「拒绝连接」/ 404 | 分两步定位：①先用 `curl --unix-socket /var/apps/SmartDisk/target/app.sock http://localhost/app/SmartDisk/api/health` 直连 socket，返回 JSON 说明应用层正常；②若直连正常但浏览器 404，是**网关注册未生效**——重装/重启应用让 fnOS 建立路由（prefix 大小写不敏感，但建议与 `appname` 一致用 `/app/SmartDisk`） |
| API 返回 `invalid token` | 正常，网关要求 NAS 登录态；浏览器已登录 iframe 打开即正常，curl 需带登录 cookie。说明网关转发已通 |
| 应用起不来 | 看 `${TRIM_PKGVAR}/smartdisk.log`；`cmd/main` 的 `TRIM_TEMP_LOGFILE` 会带错误给应用中心 |
| 列表空 | SSH 跑 `lsblk -J -O -b` 看有没有 `type=disk`；`smartdisk-cli list` 看采集是否报错 |
| 某盘「未知/SMART 不可读」 | USB 桥接不透传：`smartctl -a -j -d sat /dev/sdX` 手动试各协议 |
| 某属性显示「-」 | 该属性 ID 不在 `attrnames.go` 映射表，补一条即可 |
| 链路速率/自检/设备统计不显示 | 该盘/桥接不支持对应数据源，正常；要确认就取 `smartctl -a -j` 原始输出核对 |
| 端口被占 | 改 manifest `service_port` + `app/ui/config` + `cmd/main`，三处一致（网关模式下端口仅用于直连） |
| 前端白屏 | 浏览器 DevTools 看 `/api/disks`（网关下是 `/app/SmartDisk/api/disks`）是否 200 |
| 重新打包后图标没变 | 清 `.DS_Store` 并确认 4 个图标文件都替换（ICON.PNG/ICON_256.PNG/ui/images/icon_64\|256.png） |
| 深度下线后唤醒没补挂载 | 查 `@appdata/SmartDisk/hibernate.json` 该盘 `mounts` 是否有 UUID；md 阵列上的 fs 标记 `Noauto`（由内核重组回挂，正常）；独立盘需 `blkid` 能查到 UUID |
| 电源按钮全部禁用 | 该盘是 NVMe/SSD/USB 桥接或系统盘（tooltip 有原因）；只有 SATA 机械盘可休眠/下线 |
| 深度下线报「阵列降级」409 | 该盘是 md 成员，前端需勾选风险确认后带 `confirmRaid:true` 重试 |
| 唤醒超时 | 盘体起旋失败或 USB 桥接不响应；SSH 手动 `echo running > /sys/block/sdX/device/state` + `rescan` 排查 |
| 操作日志为空 | 日志在本版才引入，旧版本操作无记录；文件在 `@appdata/SmartDisk/power-events.json`，可直接查看 |
| 下线后「唤醒」按钮没了 / 记录丢失 | `cmd/main` 显式设 `SMARTDISK_DATA_DIR=${TRIM_PKGVAR}`（持久化）。**手动救回 offline 盘**：`echo running > /sys/block/sdX/device/state` → `sleep 8` → `echo 1 > /sys/block/sdX/device/rescan`，再用 `lsblk` 确认回来 |
| 重启后唤醒找不到盘 | 飞牛重启会给盘重新分配盘符（sda↔sdb…）。下线记录以**序列号**为 key（无序列号回退 WWN），唤醒按序列号解析当前盘符；sysfs 操作用下线时记录的盘符（offline 期间盘符不变） |

---

**关键路径速记**：后端入口 `server/main.go` → 采集 `server/internal/collector/collector.go` → 电源控制 `server/internal/collector/power.go`（持久化 `hibernate.go`、日志 `powerlog.go`）→ 解析评级 `server/internal/smart/`（解析 `smartctl.go`、中文 `attrnames.go`/`translate.go`、评级 `health.go`）→ API `server/internal/httpapi/server.go` → 前端 `web/src/views/`。

---

## 12. GitHub 仓库与在线介绍页部署

**仓库结构**：GitHub 仓库根目录就是本工程根目录（`server/`、`web/`、`SmartDisk/`、`site/` 等），`SmartDisk.fpk` 与 `node_modules`、`dist` 等构建产物不入库。本地目录如需重新接入仓库：`git init` 后把 `origin` 指向 <https://github.com/chanhuan1/Fn-SmartDisk.git>，`git fetch origin master && git checkout -f -B master origin/master` 即可（会以远端为准覆盖工作区，注意先备份本地未提交改动）。

**在线介绍页**：`site/index.html` 是纯静态介绍页，部署在 Cloudflare Pages，域名 <https://fn-smartdisk.pages.dev>。它只是项目宣传/安装指引，与 fnOS 应用本体无运行时依赖；改介绍页只需改 `site/`，push 到 master 后自动上线。

**自动部署**：`.github/workflows/deploy.yml` 在每次 push 到 `master`（或手动 workflow_dispatch）时，用 wrangler 执行 `wrangler pages deploy site --project-name fn-smartdisk --branch master`。所需 Secrets 有两个，`tools/set-secrets.mjs` 是一次性写入工具：

- `CF_API_TOKEN`：Cloudflare API Token（需 Pages 部署权限）
- `CF_ACCOUNT_ID`：Cloudflare 账户 ID

**部署排错**：Actions 失败先看 Secret 是否齐全、Token 是否过期；页面不更新时确认 workflow 是否跑完、以及浏览器缓存（介绍页为静态资源，可强刷）。`wrangler.jsonc` 仅声明 Pages 项目名与静态资源目录，本地预览可 `npx wrangler pages dev site`。
