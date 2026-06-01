# LuCI App 5echarging-go | 抑疑电止

五邑大学专用 OpenWrt LuCI 电量监控插件，用于查询和记录宿舍剩余电量，并在低电量/预计耗尽时发送邮件提醒  
*显而易见的，插件的正常运行需要设备处于五邑大学校园网中  
适用人群 : 宿舍中有 `台式机` `NAS` 甚至 `服务器(?你认真的吗` 且没有配备UPS又不希望因电费耗尽停电而损坏设备的人，亦或者你只是不想在炎炎夏日因停电没空调而被热醒(笑  
如有帮助请点个🌟!star!🌟

## 功能

- 多独立配置: 楼栋 | 宿舍号 | 备注 | 收件邮箱
- 手动查询
- 官方电费页快捷跳转
- 定时查询(整点查询/定时查询)
- 可自定义数据库路径，默认 `/etc/5echarging/5echarging.bbolt`
- 耗尽时间预测，计算依据: 自定义固定日消耗 | 根据历史记录计算日消耗
- 检测充值并按充值周期分段计算，多个有效分段按时间跨度加权平均
- 插入数据库前去重
- 每宿舍历史记录数上限，默认500
- 低电量(自定义阈值)&预计耗尽x天前发送邮件告警

## 快速开始

从本项目的 Releases 下载最新 ipk，上传至 OpenWrt 安装
随后在`服务`一栏找到`抑疑电止`，选择对应架构在线下载核心，或提前下载核心后离线上传安装(见下文)。核心就绪后，勾选启用服务，填写好参数信息后保存并应用即可。  
若安装设备不方便访问 GitHub，可用其他设备从 Releases 下载好对应的 `5echarging-linux-*` 核心文件，进入 LuCI 后使用“上传并安装核心”  
注:ipk为通用包，核心有架构区别，当前在线发布的核心架构有 `amd64`、`arm`、`arm64`、`mips`、`mipsle`，如有其他架构需求，可自行编译

![p1](/image/p1.png "界面展示")

## 数据库

默认数据库位置：

```text
/etc/5echarging/5echarging.bbolt
```

写入策略：

- 查询成功后和同一宿舍最新记录比较
- 若余额相同，且抄表时间也相同，则跳过写库
- 如果余额或抄表时间变化，则写入新历史记录
- 写入后若超出`最大历史记录数`则裁剪最旧记录

## 预测逻辑

自动预测使用最近数条历史记录(由`历史采样次数`决定)：

- 先按时间排序
- 检测余额上升作为充值点，并按充值切分为多个消耗段
- 每个分段内部计算消耗量和时间跨度
- 多个有效分段按时间跨度加权平均，得到日均消耗
- 如果配置了`自定义日消耗`，则不做预测直接使用该值

若最近样本没有观察到电量下降，预计耗尽时间会显示为不可用。这可能发生在刚充值、长期无人用电、或接口抄表数据尚未更新时

## 构建

本仓库的 ipk 是通用 LuCI 套壳包（`Architecture: all`），未内置 Go 核心二进制。构建 ipk 只需一个可用的 OpenWrt SDK (以防万一，版本尽量与您设备的openwrt版本相同或相近，架构可以任意)；核心二进制由 GitHub Actions 发布到 Releases，或本地使用 Go 交叉编译

获取与你的 OpenWrt 主版本匹配的[OpenWrt SDK](https://downloads.openwrt.org)

1. 将 SDK 解压至项目根目录
2. 在项目根目录执行：

```bash
chmod +x ./build.sh
./build.sh
```

脚本会自动打包 LuCI 套壳，完成后只需在项目根目录寻找 `luci-app-5echarging_*_all.ipk`
若报错请手动补齐依赖

如需指定 SDK 路径：

```bash
./build.sh /path/to/openwrt-sdk-xx
```

## 注意

- 卸载插件不会主动删除数据库，如有需要请手动删除
- 长时间停用后再启用，历史跨度可能影响预测，后续持续采样后预测会逐步接近实际用电速度

## 若无openwrt设备

也可以直接运行本程序，在[Releases](https://github.com/Quiet-of-far/luci-app-5echarging-go/releases)中下载或自行编译对应架构的二进制文件后，使用以下命令运行

```bash
./5echarging-* -config ./path/to/config.json
```

配置文件示例

```jsonc
{
  "rooms": [
    {
      "building": "66", // 楼栋号
      "room": "2333", // 宿舍号
      "label": "66栋2333", // 备注名
      "recipients": ["example@email.com"] // 告警接收邮箱
    }
  ],
  "schedule": {
    "interval_minutes": 60, // 轮询间隔(min)
    "check_hours": [0,14] // 指定整点检查(0点和14点)
  },
  "low_energy_threshold": 20, // 余额低于20度立刻告警
  "depletion_alert_days": 2, // 预计2天内耗尽时预警
  "max_records_per_room": 500, // 各宿舍历史记录上限
  "prediction": {
    "sample_count": 20, // 获取最近20条历史记录计算日均消耗
    "custom_daily_consumption": 0 // 覆盖日均消耗，若>0则固定使用此值预测耗尽日期
  },
  "email": {
    "enabled": true, // 是否启用邮件通知
    "alert_once_per_day": false, // 是否限制每个宿舍每日最多发送一次预警邮件
    "smtp_host": "", // SMTP服务器地址
    "smtp_port": 587, // SMTP端口
    "username": "", // 发件账号
    "password": "", // 密码/授权码
    "from": "" // 发件人显示名称/邮箱
  },
  "db_path": "/path/to/your/5echarging.bbolt" // 数据库保存路径
}
```

## 免责声明

本项目仅用于学习和研究，请勿用于非法用途  

使用者需自行承担使用本软件的所有法律责任和后果

## 开源许可

- MIT
