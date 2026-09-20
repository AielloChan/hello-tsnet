export function deviceHost(device) {
  return device.dnsName || device.addresses?.find((address) => !address.includes(":")) || device.addresses?.[0] || "";
}

export function splitTarget(target) {
  const separator = target.lastIndexOf(":");
  return { host: target.slice(0, separator).replace(/^\[|\]$/g, ""), port: target.slice(separator + 1) };
}

export function joinTarget(host, port) {
  host = host.trim().replace(/^\[|\]$/g, "");
  return `${host.includes(":") ? `[${host}]` : host}:${port}`;
}

export function protocolForRule(rule) {
  return rule?.protocol ?? (rule ? "tcp" : "both");
}

export function protocolLabel(rule) {
  return { tcp: "TCP", udp: "UDP", both: "TCP + UDP" }[protocolForRule(rule)] || "未知协议";
}

export function directionForRule(rule) {
  return rule?.direction ?? "outbound";
}

export function createRulePayload({ name, localPort, host, port, protocol, direction = "outbound" }) {
  const payload = { name, localPort: Number(localPort), protocol, direction };
  if (direction === "outbound") payload.target = joinTarget(host, port);
  return payload;
}

function normalizedHost(host) {
  host = host.toLowerCase().replace(/\.$/, "");
  if (host.includes(":")) {
    try { return new URL(`http://[${host}]`).hostname; } catch { return host; }
  }
  return host;
}

export function groupRules(devices, rules) {
  const aliases = new Map();
  const byDevice = new Map(devices.map((device) => [device.id, []]));
  for (const device of devices) {
    const hosts = [...(device.addresses || [])];
    if (device.dnsName) hosts.push(device.dnsName, device.dnsName.split(".")[0]);
    for (const host of new Set(hosts.map(normalizedHost))) {
      const owners = aliases.get(host) || new Set();
      owners.add(device.id);
      aliases.set(host, owners);
    }
  }
  const unmatched = [];
  const shared = [];
  for (const rule of rules) {
    if (directionForRule(rule) === "inbound") {
      shared.push(rule);
      continue;
    }
    const owners = aliases.get(normalizedHost(splitTarget(rule.target).host));
    if (owners?.size === 1) byDevice.get([...owners][0]).push(rule);
    else unmatched.push(rule);
  }
  return { byDevice, unmatched, shared };
}

export function nextLocalPort(rules) {
  const used = new Set(rules.map((rule) => rule.localPort));
  for (let port = 30001; port <= 65535; port++) {
    if (!used.has(port)) return port;
  }
  return "";
}

export function createRulesRefresher(load, apply, onError) {
  let sequence = 0;
  return async () => {
    const request = ++sequence;
    try {
      const rules = await load();
      if (request === sequence) apply(rules);
    } catch (error) {
      if (request === sequence) onError(error);
    }
  };
}
