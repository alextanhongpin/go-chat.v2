function onload() {
  const socket = new WebSocket(`ws://${document.location.host}/ws`);
  socket.onclose = onClose;
  socket.onmessage = onMessage;

  function onClose(evt) {
    console.log("closed", evt);
  }

  function onMessage(evt) {
    console.log("message", evt.data);
  }

  setTimeout(() => {
    const cmd = {
      msg: "hello world"
    };

    socket.send(JSON.stringify(cmd));
  }, 1000);
}

window.addEventListener("load", onload);
