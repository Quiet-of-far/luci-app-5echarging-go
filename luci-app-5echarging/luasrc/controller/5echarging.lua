module("luci.controller.5echarging", package.seeall)

local http = require("luci.http")
local jsonc = require("luci.jsonc")
local sys = require("luci.sys")
local fs = require("nixio.fs")
local util = require("luci.util")

local CORE_PATH = "/usr/bin/5echarging"
local RELEASE_BASE_URL = "https://github.com/Quiet-of-far/luci-app-5echarging-go/releases/download/v1.0.1/5echarging-linux-"

local ARCH_WHITELIST = {
	amd64 = true,
	arm = true,
	arm64 = true,
	mips = true,
	mipsle = true
}

function index()
	entry({"admin", "services", "5echarging"}, cbi("5echarging"), _("抑疑电止"), 90)
	entry({"admin", "services", "5echarging", "status"}, call("action_status")).leaf = true
	entry({"admin", "services", "5echarging", "query_now"}, call("action_query_now")).leaf = true
	entry({"admin", "services", "5echarging", "test_email"}, call("action_test_email")).leaf = true
	entry({"admin", "services", "5echarging", "download_core"}, call("action_download_core")).leaf = true
	entry({"admin", "services", "5echarging", "upload_core"}, call("action_upload_core")).leaf = true
end

function action_status()
	if service_enabled() then
		if render_status_cache() then
			return
		end
		write_json_error("服务运行中，暂无状态缓存；请等待一次定时查询或先执行手动查询")
		return
	end

	render_command("/usr/bin/5echarging -config /var/etc/5echarging.json status", true)
end

function action_query_now()
	render_query_now()
end

function action_test_email()
	render_command("/usr/bin/5echarging -config /var/etc/5echarging.json test-notify", true)
end

local function is_xhr_request()
	return http.formvalue("xhr") == "1"
end

local function redirect_with_result(ok, msg)
	local base = require("luci.dispatcher").build_url("admin", "services", "5echarging")
	local key = ok and "core_msg" or "core_err"
	http.redirect(base .. "?" .. key .. "=" .. util.urlencode(msg or ""))
end

local function respond_result(ok, msg)
	if is_xhr_request() then
		http.status(ok and 200 or 400, ok and "OK" or "Bad Request")
		http.prepare_content("text/plain; charset=utf-8")
		http.write((ok and "OK:" or "ERR:") .. (msg or ""))
		return
	end

	redirect_with_result(ok, msg)
end

local function write_json_error(message)
	http.prepare_content("application/json")
	http.write_json({
		status = "error",
		message = message
	})
end

function service_enabled()
	local uci = require("luci.model.uci").cursor()
	return uci:get("5echarging", "global", "enabled") == "1"
end

local function configured_db_path()
	local uci = require("luci.model.uci").cursor()
	return uci:get("5echarging", "global", "db_path") or "/etc/5echarging/5echarging.bbolt"
end

function render_status_cache()
	local cache_path = configured_db_path() .. ".status.json"
	local data = fs.readfile(cache_path)
	if not data or data == "" then
		return false
	end

	local payload = jsonc.parse(data)
	if not payload then
		return false
	end

	http.prepare_content("application/json")
	http.write_json(payload)
	return true
end

function render_query_now()
	local restart_service = service_enabled()

	if restart_service then
		sys.call("/etc/init.d/5echarging stop >/dev/null 2>&1")
	end

	render_command("/usr/bin/5echarging -config /var/etc/5echarging.json query-now", true)

	if restart_service then
		sys.call("/etc/init.d/5echarging start >/dev/null 2>&1")
	end
end

local function core_available()
	return fs.access(CORE_PATH, "x")
end

local function install_core_file(tmp_path)
	if not fs.access(tmp_path) then
		return false, "核心文件不存在"
	end

	if (fs.stat(tmp_path, "size") or 0) <= 0 then
		fs.remove(tmp_path)
		return false, "核心文件为空"
	end

	if sys.call("chmod 755 " .. util.shellquote(tmp_path)) ~= 0 then
		fs.remove(tmp_path)
		return false, "核心文件不可执行"
	end

	if sys.call("mv -f " .. util.shellquote(tmp_path) .. " " .. util.shellquote(CORE_PATH)) ~= 0 then
		fs.remove(tmp_path)
		return false, "安装核心失败"
	end

	return true, "核心已安装到 /usr/bin/5echarging"
end

function action_download_core()
	local arch = http.formvalue("arch") or ""

	if not ARCH_WHITELIST[arch] then
		respond_result(false, "不支持的架构: " .. arch)
		return
	end

	local url = RELEASE_BASE_URL .. arch
	local tmp_file = "/tmp/5echarging.download"
	local q_tmp = util.shellquote(tmp_file)
	local q_url = util.shellquote(url)
	local cmd = "rm -f " .. q_tmp ..
		"; (command -v uclient-fetch >/dev/null 2>&1 && uclient-fetch -qO " .. q_tmp .. " " .. q_url ..
		") || wget -qO " .. q_tmp .. " " .. q_url

	if sys.call(cmd) ~= 0 then
		fs.remove(tmp_file)
		respond_result(false, "下载失败，请检查网络或 Release 文件是否存在")
		return
	end

	local ok, msg = install_core_file(tmp_file)
	respond_result(ok, msg)
end

function action_upload_core()
	local tmp_file = "/tmp/5echarging.upload"
	local uploaded = false
	local fp = nil

	fs.remove(tmp_file)

	http.setfilehandler(function(meta, chunk, eof)
		if not fp and meta and meta.name == "core_file" then
			fp = io.open(tmp_file, "w")
		end

		if fp and chunk then
			fp:write(chunk)
		end

		if fp and eof then
			fp:close()
			uploaded = true
		end
	end)

	http.formvalue("core_file")

	if not uploaded then
		respond_result(false, "未检测到上传文件")
		return
	end

	local ok, msg = install_core_file(tmp_file)
	respond_result(ok, msg)
end

function render_command(command, regenerate_config)
	if not core_available() then
		write_json_error("未检测到 /usr/bin/5echarging，请先下载或上传核心")
		return
	end

	if regenerate_config then
		sys.call("/usr/bin/5echarging-uci2json >/dev/null 2>&1")
	end

	local output = sys.exec(command)
	local payload = jsonc.parse(output)

	http.prepare_content("application/json")
	if payload then
		http.write_json(payload)
		return
	end

	http.write_json({
		status = "error",
		message = output ~= "" and output or "命令执行失败"
	})
end
