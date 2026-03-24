(() => {
  const terminalElement = document.getElementById("terminal");
  if (!terminalElement) {
    return;
  }

  const terminalTheme = {
    background: "#000000",
    foreground: "#f5f5f5",
    cursor: "#f5f5f5",
    cursorAccent: "#000000",
    selectionBackground: "rgba(255,255,255,0.18)",
    black: "#1f1f1f",
    red: "#ff6b6b",
    green: "#78d381",
    yellow: "#f5d76e",
    blue: "#75b7ff",
    magenta: "#c89dff",
    cyan: "#7ee7e7",
    white: "#e7e7e7",
    brightBlack: "#6f6f6f",
    brightRed: "#ff8e8e",
    brightGreen: "#9cef9c",
    brightYellow: "#ffe28f",
    brightBlue: "#9ecbff",
    brightMagenta: "#d8bbff",
    brightCyan: "#9ff4f4",
    brightWhite: "#ffffff",
  };

  const textDecoder = new TextDecoder("utf-8");
  const textEncoder = new TextEncoder();
  let decoderFlushed = true;

  function base64ToBytes(base64) {
    const binString = atob(base64);
    const bytes = new Uint8Array(binString.length);
    for (let i = 0; i < binString.length; i++) {
      bytes[i] = binString.charCodeAt(i);
    }
    return bytes;
  }

  function bytesToBase64(bytes) {
    let binString = "";
    for (let i = 0; i < bytes.length; i++) {
      binString += String.fromCharCode(bytes[i]);
    }
    return btoa(binString);
  }

  function writeChunk(bytes, flush = false) {
    if (typeof terminal.writeUtf8 === "function") {
      terminal.writeUtf8(bytes);
      return;
    }

    const text = textDecoder.decode(bytes, { stream: !flush });
    if (text) {
      terminal.write(text);
    }
    decoderFlushed = flush;
  }

  function flushDecoder() {
    if (decoderFlushed) {
      return;
    }
    const tail = textDecoder.decode();
    if (tail) {
      terminal.write(tail);
    }
    decoderFlushed = true;
  }

  const FitAddonCtor = window.FitAddon && (window.FitAddon.FitAddon || window.FitAddon.default || window.FitAddon);
  const fitAddon = new FitAddonCtor();
  const terminal = new window.Terminal({
    allowTransparency: false,
    convertEol: false,
    cursorBlink: true,
    cursorStyle: "block",
    drawBoldTextInBrightColors: true,
    fontFamily: '"SF Mono", "JetBrains Mono", "Menlo", "Monaco", monospace',
    fontSize: 14,
    lineHeight: 1.35,
    macOptionIsMeta: true,
    scrollback: 5000,
    theme: terminalTheme,
  });

  terminal.loadAddon(fitAddon);
  terminal.open(terminalElement);
  fitAddon.fit();
  terminal.focus();

  let socket = null;
  let reconnectTimer = null;
  let reconnectAttempts = 0;
  let resizeFrame = 0;

  function setState(value) {
    document.title = "Claude Code" + (value ? " • " + value : "");
  }

  function sendResize() {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return;
    }
    fitAddon.fit();
    socket.send(JSON.stringify({
      type: "resize",
      cols: terminal.cols,
      rows: terminal.rows,
    }));
  }

  function connect() {
    if (socket && socket.readyState === WebSocket.OPEN) {
      return;
    }

    fitAddon.fit();
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const params = new URLSearchParams({
      cols: String(terminal.cols),
      rows: String(terminal.rows),
    });

    socket = new WebSocket(protocol + "://" + window.location.host + "/ws/terminal?" + params.toString());
    setState("CONNECTING");

    socket.addEventListener("open", () => {
      reconnectAttempts = 0;
      setState("CONNECTED");
      terminal.focus();
      sendResize();
    });

    socket.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);
      switch (message.type) {
        case "snapshot":
          flushDecoder();
          terminal.reset();
          if (message.data) {
            writeChunk(base64ToBytes(message.data), true);
          }
          setState(message.running ? "CONNECTED" : "IDLE");
          break;
        case "output":
          if (message.data) {
            writeChunk(base64ToBytes(message.data));
          }
          break;
        case "status":
          setState(message.running ? "CONNECTED" : "IDLE");
          break;
        case "error":
          setState("ERROR");
          if (message.message) {
            terminal.writeln("\r\n[web-claude] " + message.message);
          }
          break;
        default:
          break;
      }
    });

    socket.addEventListener("close", () => {
      flushDecoder();
      setState("DISCONNECTED");
      socket = null;
      reconnectAttempts += 1;
      clearTimeout(reconnectTimer);
      reconnectTimer = window.setTimeout(connect, Math.min(5000, reconnectAttempts * 1000));
    });
  }

  terminal.onData((data) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return;
    }
    const bytes = textEncoder.encode(data);
    socket.send(JSON.stringify({
      type: "input",
      data: bytesToBase64(bytes),
    }));
  });

  window.addEventListener("resize", () => {
    if (resizeFrame) {
      window.cancelAnimationFrame(resizeFrame);
    }
    resizeFrame = window.requestAnimationFrame(() => {
      resizeFrame = 0;
      sendResize();
    });
  });
  connect();
})();
