function sendMessage(message) {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendMessage(message, (response) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
        return;
      }
      if (!response?.ok) {
        reject(new Error(response?.error || "Extension request failed"));
        return;
      }
      resolve(response.result);
    });
  });
}

const urlOutput = document.getElementById("workspace-url");
const connectionState = document.getElementById("connection-state");
const checkButton = document.getElementById("check");
const disconnectButton = document.getElementById("disconnect");
const savePromptsInput = document.getElementById("credential-save-prompts");
const status = document.getElementById("status");

async function loadConfig() {
  try {
    const config = await sendMessage({ type: "get-config" });
    urlOutput.textContent = config.workspaceBaseUrl || "Not connected";
    connectionState.textContent = config.sessionToken ? "Connected session stored" : "Waiting for connection";
    savePromptsInput.checked = config.credentialSavePromptsEnabled === true;
  } catch (error) {
    status.textContent = error instanceof Error ? error.message : "Loading extension settings failed.";
  }
}

checkButton.addEventListener("click", async () => {
  status.textContent = "Checking workspace session...";
  try {
    const result = await sendMessage({ type: "check-auth" });
    urlOutput.textContent = result.workspaceBaseUrl || "Not connected";
    if (result.connected) {
      connectionState.textContent = `Signed in as ${result.user.name}`;
      status.textContent = `Connected to ${result.workspaceBaseUrl} as ${result.user.name} (${result.authMode}).`;
      return;
    }
    connectionState.textContent = "Waiting for connection";
    status.textContent = "Open Access Workspace in this browser and use Connect extension.";
  } catch (error) {
    status.textContent = error instanceof Error ? error.message : "Checking the workspace session failed.";
  }
});

disconnectButton.addEventListener("click", async () => {
  status.textContent = "Disconnecting extension...";
  try {
    await sendMessage({ type: "disconnect-workspace" });
    await loadConfig();
    connectionState.textContent = "Waiting for connection";
    status.textContent = "Extension disconnected.";
  } catch (error) {
    status.textContent = error instanceof Error ? error.message : "Disconnecting the workspace session failed.";
  }
});

savePromptsInput.addEventListener("change", async () => {
  const enabled = savePromptsInput.checked;
  savePromptsInput.disabled = true;
  status.textContent = enabled
    ? "Credential saving prompts enabled."
    : "Credential saving prompts disabled.";
  try {
    await sendMessage({ type: "set-credential-save-prompts-enabled", enabled });
  } catch (error) {
    savePromptsInput.checked = !enabled;
    status.textContent = error instanceof Error ? error.message : "Updating credential saving failed.";
  } finally {
    savePromptsInput.disabled = false;
  }
});

void loadConfig();
