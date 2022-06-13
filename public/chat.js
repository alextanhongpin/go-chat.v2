import Service from "./service.js";

const $ = (el) => document.getElementById(el);

async function onload() {
  let friends = [];
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

  function send(type, { text, presence } = {}) {
    socket.send(JSON.stringify({ type, text, presence }));
  }

  function onClose(evt) {
    $("output").innerText = evt.reason;
  }

  function onMessage(evt) {
    const event = JSON.parse(evt.data);
    eventProcessor(event);
  }

  const eventHandlers = {
    presence: presenceNotified,
    text: messageSent,
    friends: friendsFetched,
  };

  function messageSent({ type, text }) {
    $("output").innerText += "\n";
    $("output").innerText += text;
  }

  function presenceNotified({ from, presence }) {
    friends = friends.map((friend) =>
      friend.username === from
        ? {
            username: from,
            online: presence,
          }
        : friend
    );
    renderFriends();
  }

  function friendsFetched({ friends: onlineFriends = [] } = {}) {
    friends = onlineFriends;
    renderFriends();
  }
  function renderFriends() {
    $("aside").innerHTML = friends
      .map(({ username, online }) => {
        return `<div>${username} ${online ? "online" : "offline"}</div>`;
      })
      .join("\n");
  }

  function eventProcessor(event) {
    const handler = eventHandlers[event.type];
    if (!handler) {
      throw new Error(`not implemented: ${event.type}`);
    }

    return handler(event);
  }

  $("chat").addEventListener(
    "keyup",
    (evt) => {
      const isEnter = evt.which === 13;
      const message = $("chat").value;
      if (isEnter && message) {
        send("text", { text: message });
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
        send("text", { text: message });
        $("chat").value = "";
      }
    },
    false
  );
}

window.addEventListener("load", onload);
