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

async function api(path, options = {}) {
  const response = await fetch(path, {
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

async function refreshRules() {
  rules = await api("/api/rules");
  tableBody.replaceChildren(...rules.map(ruleRow));
  table.hidden = rules.length === 0;
  empty.hidden = rules.length !== 0;
}

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
    await api(`/api/rules/${encodeURIComponent(rule.id)}`, { method: "DELETE" });
    await refreshRules();
  };
  actions.append(edit, remove);
  row.append(name, local, target, actions);
  return row;
}

function openForm(rule) {
  $("#form-title").textContent = rule ? "编辑映射" : "新增映射";
  $("#rule-id").value = rule?.id || "";
  $("#name").value = rule?.name || "";
  $("#port").value = rule?.localPort || "";
  $("#target").value = rule?.target || "";
  $("#form-error").textContent = "";
  dialog.showModal();
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const id = $("#rule-id").value;
  const payload = JSON.stringify({
    name: $("#name").value,
    localPort: Number($("#port").value),
    target: $("#target").value,
  });
  try {
    await api(id ? `/api/rules/${encodeURIComponent(id)}` : "/api/rules", { method: id ? "PUT" : "POST", body: payload });
    dialog.close();
    await refreshRules();
  } catch (error) {
    $("#form-error").textContent = error.message;
  }
});

$("#new-rule").onclick = () => openForm(null);
$("#cancel").onclick = () => dialog.close();
$("#close-dialog").onclick = () => dialog.close();

await Promise.all([refreshStatus(), refreshRules()]);
setInterval(refreshStatus, 2000);
