local m, s, o
local sys = require "luci.sys"
local uci = require "luci.model.uci".cursor()

m = Map("5echarging", "抑疑电止", "宿舍剩余电量自动查询、预测与预警")

function m.on_after_commit(self)
	sys.call("/usr/bin/5echarging-uci2json >/dev/null 2>&1")

	if uci:get("5echarging", "global", "enabled") == "1" then
		sys.call("/etc/init.d/5echarging enable >/dev/null 2>&1")
		sys.call("/etc/init.d/5echarging restart >/dev/null 2>&1")
	else
		sys.call("/etc/init.d/5echarging stop >/dev/null 2>&1")
		sys.call("/etc/init.d/5echarging disable >/dev/null 2>&1")
	end
end

s = m:section(SimpleSection)
s.template = "5echarging/status"

-- 基本设置
s = m:section(NamedSection, "global", "5echarging", "基本设置")
s.anonymous = true
s.addremove = false

o = s:option(Flag, "enabled", "启用服务")
o.rmempty = false

o = s:option(Value, "low_energy_threshold", "低电量预警（度）", "剩余电量低于此值时发送提醒")
o.datatype = "ufloat"
o.default = "15"

o = s:option(Value, "depletion_alert_days", "耗尽前预警（天）", "预计耗尽时间在此天数内时发送提醒")
o.datatype = "uinteger"
o.default = "0"

o = s:option(Value, "db_path", "数据库路径")
o.default = "/etc/5echarging/5echarging.bbolt"

o = s:option(Value, "max_records_per_room", "最大历史记录数（每宿舍）", "超过此数量时删除该宿舍最早的历史记录；设为 0 表示不限制")
o.datatype = "uinteger"
o.default = "500"

-- 定时任务
s = m:section(NamedSection, "schedule", "schedule", "定时任务")
s.anonymous = true
s.addremove = false

o = s:option(Value, "interval_minutes", "查询间隔（分钟）", "当未设置固定查询时间时，按此间隔轮询")
o.datatype = "uinteger"
o.default = "60"

o = s:option(DynamicList, "check_hours", "每日查询时间（整点）", "填入 0-23 的整数，例如 8、12、18、22")
o.datatype = "range(0,23)"

-- 宿舍列表
s = m:section(TypedSection, "room", "宿舍列表")
s.anonymous = true
s.addremove = true
s.template = "cbi/tblsection"

o = s:option(Value, "building", "楼栋号")
o.rmempty = false

o = s:option(Value, "room", "宿舍号")
o.rmempty = false

o = s:option(Value, "label", "备注名称")

o = s:option(DynamicList, "recipients", "收件人地址")

-- 电量预测
s = m:section(NamedSection, "prediction", "prediction", "电量耗尽预测")
s.anonymous = true
s.addremove = false

o = s:option(Value, "sample_count", "历史采样次数", "用于计算消耗速度的近期记录条数")
o.datatype = "uinteger"
o.default = "10"

o = s:option(Value, "custom_daily_consumption", "自定义日消耗（度/天）", "设为 0 则自动计算")
o.datatype = "ufloat"
o.default = "0"

-- 邮件通知
s = m:section(NamedSection, "email", "email", "邮件通知")
s.anonymous = true
s.addremove = false

o = s:option(Flag, "enabled", "启用邮件通知")
o.rmempty = false

o = s:option(Flag, "alert_once_per_day", "邮件告警 1 日至多 1 次", "启用后每个宿舍每天最多发送一次预警邮件")
o.rmempty = false
o.default = "0"
o:depends("enabled", "1")

o = s:option(Value, "smtp_host", "SMTP 服务器")
o.placeholder = "smtp.example.com"
o:depends("enabled", "1")

o = s:option(Value, "smtp_port", "SMTP 端口")
o.datatype = "port"
o.default = "587"
o:depends("enabled", "1")

o = s:option(Value, "username", "用户名")
o:depends("enabled", "1")

o = s:option(Value, "password", "密码")
o.password = true
o:depends("enabled", "1")

o = s:option(Value, "from_addr", "发件人地址")
o:depends("enabled", "1")

return m
