import { message } from "/message.js";
import "/style.css";

const messageElement = document.querySelector("#message");
const routeElement = document.querySelector("#route");

const renderMessage = (value) => {
  messageElement.textContent = value;
};

const renderRoute = () => {
  routeElement.textContent = window.location.pathname;
};

renderMessage(message);
renderRoute();

document.querySelector("#push-route").addEventListener("click", () => {
  window.history.pushState({}, "", "/nested/route");
  renderRoute();
});

if (import.meta.hot) {
  import.meta.hot.accept("/message.js", (module) => {
    renderMessage(module.message);
  });
}
