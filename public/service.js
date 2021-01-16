import { authorize, authenticate } from "./api.js";
import { safeRedirect } from "./route.js";

export default class Service {
  async authorize() {
    try {
      // Attempt to authorize the user.
      const username = await authorize();

      // Successful authorization, redirect to private route /chat.
      // If the user is already there, return the username.
      if (safeRedirect("/chat.html")) {
        return username;
      }
    } catch (error) {
      // Remove the token.
      window.localStorage.removeItem("accessToken");

      // Redirect to root for login.
      // If the user is already there, throw the error.
      if (safeRedirect("/")) {
        throw error;
      }
    }
  }

  async authenticate(username) {
    await authenticate(username);
    await this.authorize();
  }

  async logout() {
    window.localStorage.removeItem("accessToken");
    await this.authorize();
  }
}
