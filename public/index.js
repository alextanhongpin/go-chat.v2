import connect from "./socket.js";
import Service from "./service.js";

const $ = el => document.getElementById(el);

function showError(error) {
  $("output").innerText = error.message;
}
async function onload() {
  const service = new Service();
  try {
    await service.authorize();
  } catch (error) {
    showError(error);
  }

  $("submit").addEventListener(
    "click",
    async () => {
      try {
        await service.authenticate($("username").value);
      } catch (error) {
        showError(error);
      }
    },
    false
  );
}

window.addEventListener("load", onload);
