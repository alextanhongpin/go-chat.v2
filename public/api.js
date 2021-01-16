export async function authenticate(username) {
  const body = await window.fetch("/authenticate", {
    method: "POST",
    body: JSON.stringify({
      username
    })
  });

  const response = await body.json();
  window.localStorage.accessToken = response.accessToken;
}

export async function authorize() {
  const accessToken = window.localStorage.accessToken;
  console.log("autho", accessToken);
  if (!accessToken) {
    throw new Error("accessToken is required");
  }

  const body = await window.fetch("/authorize", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${accessToken}`
    }
  });
  const response = await body.json();
  return response.username;
}
