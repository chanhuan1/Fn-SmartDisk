#!/bin/bash
# fnOS mobile App can navigate to the gateway before its WebView has received
# the current NAS session cookie. This fixed CGI page waits for the gateway
# session to become usable, then enters the normal /app/SmartDisk route.
set -euo pipefail

cat <<'HTML'
Content-Type: text/html; charset=utf-8
Cache-Control: no-store
X-Content-Type-Options: nosniff

<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
  <meta name="color-scheme" content="dark">
  <title>磁盘高级管理</title>
  <style>
    :root { color-scheme: dark; }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 24px;
      color: #e8eef5;
      background: #0e1319;
      font: 15px/1.6 "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
    }
    main {
      width: min(420px, 100%);
      padding: 24px;
      text-align: center;
      border: 1px solid #232d3a;
      border-radius: 10px;
      background: #161d26;
    }
    h1 { margin: 0 0 10px; font-size: 18px; }
    p { margin: 0; color: #9fabb9; }
    button {
      margin-top: 18px;
      padding: 8px 18px;
      border: 1px solid #3fc1d4;
      border-radius: 7px;
      color: #052730;
      background: #3fc1d4;
      font: inherit;
      font-weight: 700;
      cursor: pointer;
    }
  </style>
</head>
<body>
  <main>
    <h1>磁盘高级管理</h1>
    <p id="status">正在同步飞牛登录状态…</p>
    <button id="retry" type="button" hidden>重新检测</button>
  </main>
  <script>
  (() => {
    const status = document.getElementById("status");
    const retry = document.getElementById("retry");
    const target = "/app/SmartDisk/";
    const health = "/app/SmartDisk/api/health";
    const maxAttempts = 25;
    let attempts = 0;

    async function check() {
      retry.hidden = true;
      status.textContent = "正在同步飞牛登录状态…";
      try {
        const response = await fetch(`${health}?_=${Date.now()}`, {
          cache: "no-store",
          credentials: "same-origin",
          headers: { Accept: "application/json" }
        });
        const body = await response.text();
        if (response.ok && !/invalid token/i.test(body)) {
          location.replace(`${target}?_=${Date.now()}`);
          return;
        }
        if (response.status === 403) {
          status.textContent = "需要飞牛管理员权限，请使用管理员账号登录后重试。";
          retry.hidden = false;
          return;
        }
      } catch {
        // The WebView may still be connecting; retry briefly.
      }

      attempts += 1;
      if (attempts < maxAttempts) {
        setTimeout(check, 400);
        return;
      }
      status.textContent = "登录状态尚未同步，请重新登录飞牛后点击重试。";
      retry.hidden = false;
    }

    retry.addEventListener("click", () => {
      attempts = 0;
      check();
    });
    check();
  })();
  </script>
</body>
</html>
HTML
