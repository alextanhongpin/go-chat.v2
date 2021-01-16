import connect from "./socket.js";
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

  connect(window.localStorage.accessToken);
}

window.addEventListener("load", onload);
