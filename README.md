# 磁盘高级管理（SmartDisk）

扩展飞牛 fnOS 官方基础磁盘显示的原生应用，提供更完整、更专业的硬盘健康管理与冷备盘电源控制。以官方 `fnpack` 打包为 `.fpk`，在 fnOS 桌面以窗口形式打开。

## 功能

**健康检测**

- 全介质覆盖：SATA 机械盘 / 固态盘、NVMe 固态盘、USB 桥接盘（含读卡器），自动识别接口与桥接芯片并选择正确透传协议
- 剩余写入寿命：NVMe 直接读取百分比；SATA SSD 按主控厂商精准识别（群联 / 慧荣 / Intel / 三星等），三星盘按官方 TBW 估算并标注来源
- SSD 主控识别：群联 S11/S12、慧荣 SM22xx、三星、海力士、东芝/铠侠、闪迪、金士顿等
- 完整中文 SMART 属性表（含厂商专属名与关键指标标记）、自检历史、ATA 错误日志、设备统计、SCT 温度历史、磁头加载计数
- 链路诊断：SATA 协商速率与降速预警、SATA PHY 链路错误、NVMe PCIe 链路、btrfs 存储池错误计数（数据静默损坏早期信号）
- 中文健康评级：综合重映射扇区、待处理扇区、CRC 错误、温度、寿命、btrfs 错误，给出 优 / 良 / 警 / 危 与可读风险提示

**电源控制（仅 SATA 机械硬盘）**

- 深度下线：卸载卷 → 停转 → 内核级 `device offline`，飞牛后台与文件管理器都无法唤醒，冷备盘利器
- 手动唤醒：恢复 running → 重扫总线 → 自动补挂载，实时步骤流式输出（SSE）
- 保护机制：系统盘与 md 阵列成员盘需二次确认，正在读写的盘需独立确认风险
- 操作日志：环形保留最近 200 条，持久化到应用数据目录，支持一键清空

**磁盘备注**

- 给每块盘写备注（如「冷备仓库盘 · 仓库 3 号位」），按序列号持久化，重启、换槽位不丢失
- 总览卡片、详情页、电源控制页三处同步显示，总览卡片支持原地编辑

**休眠盘安全采集**

- 首次采集先探测机械盘电源状态，休眠或状态无法确认的盘只显示基础信息，读取 SMART 前由用户确认

## 安装

1. 在 fnOS 应用中心打开「手动安装」，上传 `SmartDisk/SmartDisk.fpk`
2. 安装后桌面出现「磁盘高级管理」入口，通过统一网关 `/app/SmartDisk` 访问
3. 也可通过独立端口 `8315` 直连（与网关模式数据一致）

> SMART 数据采集为只读。电源控制功能会执行停转、卸载和内核上下线操作，读取休眠盘 SMART 前会提示确认。

## 本地开发

需要 Go 1.24+ 与 Node 18+。

```bash
# 1. 构建前端（输出到 server/web/dist，被 Go embed）
cd web && npm install && npm run typecheck && npm run build

# 2. 起后端（mock 模式演示数据）
cd ../server && SMARTDISK_MOCK_DIR=./mock go run .
# 打开 http://localhost:8315

# 或前端热更开发（代理 /api 到 8315）
cd web && npm run dev
```

## 打包 .fpk

需要 [fnpack](https://developer.fnnas.com/docs/cli/fnpack/)（飞牛官方打包工具）与 Go 1.24+。

```bash
# 版本号唯一来源是 SmartDisk/manifest，需与 web/package.json 保持一致
GO=/path/to/go scripts/build.sh
# 产物：SmartDisk/SmartDisk.fpk
```

打包脚本会依次执行：前端类型检查 → 前端构建 → Linux amd64 静态交叉编译 Go 后端 → 同步 `app/www` → 暂存可选的静态 `smartctl` / `nvme`（放入 `tools/bin/` 即自动打包）→ `fnpack build`。

## 目录结构

```
SmartDisk/          fpk 应用骨架（manifest / cmd / config / app）
  app/server/       编译产物 smartdisk 二进制
  app/www/          前端构建产物（同时嵌入二进制）
  app/bin/          smartdisk-cli 启动器（可选静态 smartctl / nvme 也放这里）
  app/ui/           fnOS 桌面入口配置
server/             Go 后端源码
  internal/...      collector / smart / httpapi / model / config
  mock/             mock 采集数据（本地开发用）
  web/dist          前端构建输出（被 embed）
web/                前端源码（Vue 3 + Vite + TypeScript）
scripts/            构建 / 开发脚本
tools/              本地工具链（不入库）
```

## 权限说明

读取 SMART 需要对 `/dev/sdX`、`/dev/nvmeX` 的原始访问，因此安装版按官方「访问包用户无法处理的设备」场景以 root 运行（`config/privilege: run-as=root`），并通过 fnOS 统一网关提供桌面入口。独立 TCP 模式使用 `SMARTDISK_HTTP_ADDR` 配置监听地址，默认 `8315`。

命令行 `smartdisk-cli list` 和 `get <id>` 会安全地暂缓休眠机械盘的 SMART 读取；`smartdisk-cli scan` 会执行完整采集，可能唤醒休眠盘。

## 路线图

- [ ] SMART 自检（short / long）触发与进度
- [ ] 定时巡检 + 异常告警（fnOS 通知中心 / webhook）
- [ ] 温度与寿命历史曲线
- [ ] ARM 架构支持
