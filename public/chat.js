import Service from "./service.js";

const $ = el => document.getElementById(el);

async function onload() {
  const service = new Service();
  const username = await service.authorize();
  $("output").innerText = `Hi ${username}`;

  $("logout").addEventListener(
    "click",
    async () => {
      await service.logout();
    },
    false
  );

  const token = window.localStorage.accessToken;
  const socket = new WebSocket(
    `ws://${document.location.host}/ws?token=${token}`
  );
  socket.onclose = onClose;
  socket.onmessage = onMessage;

  function send(json) {
    socket.send(JSON.stringify(json));
  }

  function onClose(evt) {
    $("output").innerText = evt.reason;
  }

  function onMessage(evt) {
    const event = JSON.parse(evt.data);
    $("output").innerText += "\n";
    $("output").innerText += event.msg;
  }

  $("chat").addEventListener(
    "keyup",
    evt => {
      const isEnter = evt.which === 13;
      const message = $("chat").value;
      if (isEnter && message) {
        send({ msg: message });
        $("chat").value = "";
      }
    },
    false
  );

  $("submit").addEventListener(
    "click",
    () => {
      const message = $("chat").value;
      if (message) {
        send({ msg: message });
        $("chat").value = "";
      }
    },
    false
  );
}

window.addEventListener("load", onload);
