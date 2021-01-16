export default function connect(token) {
  const socket = new WebSocket(
    `ws://${document.location.host}/ws?token=${token}`
  );
  socket.onclose = onClose;
  socket.onmessage = onMessage;

  function onClose(evt) {
    console.log("closed", evt.reason);
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
