import { deviceHost, splitTarget, joinTarget, groupRules, nextLocalPort, createRulesRefresher } from "./devices.js";

const $ = (selector) => document.querySelector(selector);
const statusNode = $("#status");
const authCard = $("#auth-card");
const authLink = $("#auth-link");
const table = $("#rules-table");
const tableBody = $("#rules");
const empty = $("#empty");
const dialog = $("#rule-dialog");
const form = $("#rule-form");
let rules = [];
let devices = [];
let devicesAvailable = false;
let devicesLoading = false;
let deviceError = "";
let selectedDevice = null;
let saving = false;
let renderedDevices = "";
const expandedDevices = new Map();

async function api(path, options = {}) {
  const response = await fetch(path, {
    signal: AbortSignal.timeout(8000),
    ...options,
    headers: options.body ? { "Content-Type": "application/json", ...(options.headers || {}) } : options.headers,
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error || `请求失败 (${response.status})`);
  }
  return response.status === 204 ? null : response.json();
}

async function refreshStatus() {
  try {
    const status = await api("/api/status", { cache: "no-store" });
    statusNode.className = `status ${status.state}`;
    statusNode.textContent = status.state === "ready" ? "已连接" : status.state === "error" ? "连接错误" : "等待授权";
    authCard.hidden = !(status.state === "pending" && status.authUrl);
    if (!authCard.hidden) authLink.href = status.authUrl;
    if (status.state === "error" && status.error) statusNode.title = status.error;
  } catch (error) {
    statusNode.className = "status error";
    statusNode.textContent = "状态不可用";
    statusNode.title = error.message;
  }
}

const refreshRules = createRulesRefresher(
  () => api("/api/rules", { cache: "no-store" }),
  (items) => {
    rules = items;
    $("#page-error").textContent = "";
    renderDevices();
  },
  (error) => {
    $("#page-error").textContent = `映射列表更新失败：${error.message}`;
  }
);

function ruleRow(rule) {
  const row = document.createElement("tr");
  const name = document.createElement("td");
  name.textContent = rule.name;
  const local = document.createElement("td");
  const localCode = document.createElement("code");
  localCode.textContent = `127.0.0.1:${rule.localPort}`;
  local.append(localCode);
  const target = document.createElement("td");
  const targetCode = document.createElement("code");
  targetCode.textContent = rule.target;
  target.append(targetCode);
  const actions = document.createElement("td");
  actions.className = "row-actions";
  const edit = document.createElement("button");
  edit.textContent = "编辑";
  edit.onclick = () => openForm(rule);
  const remove = document.createElement("button");
  remove.className = "danger";
  remove.textContent = "删除";
  remove.onclick = async () => {
    if (!confirm(`删除“${rule.name}”？`)) return;
    remove.disabled = true;
    try {
      await api(`/api/rules/${encodeURIComponent(rule.id)}`, { method: "DELETE" });
      await refreshRules();
    } catch (error) {
      $("#page-error").textContent = `删除失败：${error.message}`;
    } finally {
      remove.disabled = false;
    }
  };
  actions.append(edit, remove);
  row.append(name, local, target, actions);
  return row;
}

function openForm(rule, device = null) {
  selectedDevice = device;
  const target = rule ? splitTarget(rule.target) : { host: device ? deviceHost(device) : "", port: "" };
  $("#form-title").textContent = rule ? "编辑映射" : "新增映射";
  $("#rule-id").value = rule?.id || "";
  $("#name").value = rule?.name || device?.name || "";
  $("#port").value = rule?.localPort || nextLocalPort(rules);
  $("#target-host").value = target.host;
  $("#target-port").value = target.port;
  $("#service").value = ["22", "5900", "3389"].includes(target.port) ? target.port : "";
  $("#selected-device").hidden = !device;
  $("#selected-device").textContent = device ? `为 ${device.name} 添加 TCP 转发` : "";
  $("#form-error").textContent = "";
  dialog.showModal();
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (saving) return;
  const id = $("#rule-id").value;
  const payload = JSON.stringify({
    name: $("#name").value,
    localPort: Number($("#port").value),
    target: joinTarget($("#target-host").value, $("#target-port").value),
  });
  saving = true;
  const saveButton = form.querySelector('[type="submit"]');
  saveButton.disabled = true;
  try {
    await api(id ? `/api/rules/${encodeURIComponent(id)}` : "/api/rules", { method: id ? "PUT" : "POST", body: payload });
    dialog.close();
    await refreshRules();
  } catch (error) {
    $("#form-error").textContent = error.message;
  } finally {
    saving = false;
    saveButton.disabled = false;
  }
});

$("#new-rule").onclick = () => openForm(null);
$("#cancel").onclick = () => dialog.close();
$("#close-dialog").onclick = () => dialog.close();

$("#service").onchange = () => {
  const port = $("#service").value;
  if (!port) return;
  $("#target-port").value = port;
  if (!$("#rule-id").value) {
    const serviceName = { "22": "SSH", "5900": "屏幕共享", "3389": "远程桌面" }[port];
    $("#name").value = selectedDevice ? `${selectedDevice.name} · ${serviceName}` : serviceName;
  }
};
$("#target-port").oninput = () => { $("#service").value = ""; };

async function refreshDevices() {
  if (devicesLoading) return;
  devicesLoading = true;
  $("#refresh-devices").disabled = true;
  try {
    devices = await api("/api/devices", { cache: "no-store" });
    devicesAvailable = true;
    deviceError = "";
  } catch (error) {
    devicesAvailable = false;
    deviceError = `${error.message}。${devices.length ? "以下保留上次设备列表，当前状态未知。" : ""}`;
  } finally {
    devicesLoading = false;
    $("#refresh-devices").disabled = false;
    renderDevices();
  }
}

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function deviceCard(device, attachedRules) {
  const card = element("article", "device");
  const heading = element("div", "device-heading");
  const identity = element("div", "device-identity");
  const avatar = element("span", "device-avatar", device.os === "macOS" ? "Mac" : device.os === "windows" ? "Win" : device.os === "linux" ? "Lin" : "TS");
  avatar.setAttribute("aria-hidden", "true");
  const info = element("div", "device-info");
  const name = element("div", "device-name");
  const state = devicesAvailable ? device.state : "unknown";
  const states = { online: "在线", offline: "离线", expired: "授权已过期", unknown: "状态未知" };
  name.append(element("h3", "", device.name), element("span", `device-state ${state}`, states[state] || states.unknown));
  const address = device.addresses?.join(" · ") || "暂无地址";
  info.append(name, element("p", "device-meta", `${device.os || "未知系统"} · ${address}`));
  if (device.dnsName) info.append(element("p", "device-dns", device.dnsName));
  if (devicesAvailable && device.lastSeen) {
    info.append(element("p", "device-dns", `最后在线：${new Date(device.lastSeen).toLocaleString("zh-CN")}`));
  }
  identity.append(avatar, info);
  const add = element("button", "secondary", "添加转发");
  add.type = "button";
  add.disabled = !deviceHost(device);
  add.setAttribute("aria-label", `为 ${device.name} 添加转发`);
  add.onclick = () => openForm(null, device);
  heading.append(identity, add);
  card.append(heading);
  if (attachedRules.length > 0) {
    const details = element("details", "device-mappings");
    details.open = expandedDevices.get(device.id) ?? true;
    details.ontoggle = () => expandedDevices.set(device.id, details.open);
    details.append(element("summary", "", `${attachedRules.length} 条 TCP 映射`));
    const wrapper = element("div", "table-wrap");
    const deviceTable = element("table");
    deviceTable.setAttribute("aria-label", `${device.name} 的映射`);
    const head = element("thead");
    const header = element("tr");
    for (const label of ["名称", "本机地址", "目标", "操作"]) header.append(element("th", "", label));
    head.append(header);
    const body = element("tbody");
    body.append(...attachedRules.map(ruleRow));
    deviceTable.append(head, body);
    wrapper.append(deviceTable);
    details.append(wrapper);
    card.append(details);
  }
  return card;
}

function renderDevices() {
  const query = $("#device-search").value.trim().toLowerCase();
  const signature = JSON.stringify([devices, rules, devicesAvailable, deviceError, query]);
  if (signature === renderedDevices) return;
  renderedDevices = signature;
  const grouped = groupRules(devices, rules);
  const visible = devices.filter((device) => [device.name, device.dnsName, device.os, ...(device.addresses || [])].join(" ").toLowerCase().includes(query));
  $("#devices").replaceChildren(...visible.map((device) => deviceCard(device, grouped.byDevice.get(device.id))));
  $("#device-count").textContent = devicesAvailable ? `${devices.length} 台 · ${devices.filter((device) => device.state === "online").length} 在线` : "";
  const notice = $("#devices-notice");
  notice.textContent = deviceError || (devicesAvailable ? (devices.length === 0 ? "当前没有可见设备。" : visible.length === 0 ? "没有匹配的设备。" : "") : "正在读取设备…");
  notice.hidden = !notice.textContent;
  notice.classList.toggle("warning", !devicesAvailable && !!deviceError);
  tableBody.replaceChildren(...grouped.unmatched.map(ruleRow));
  table.hidden = grouped.unmatched.length === 0;
  empty.hidden = grouped.unmatched.length !== 0;
}

$("#device-search").oninput = renderDevices;
$("#refresh-devices").onclick = async () => { await Promise.all([refreshDevices(), refreshRules()]); };

await Promise.all([refreshStatus(), refreshRules(), refreshDevices()]);
setInterval(refreshStatus, 2000);
setInterval(() => { if (!document.hidden) refreshDevices(); }, 10000);
