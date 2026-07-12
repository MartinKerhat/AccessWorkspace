(function contentScript() {
  const PANEL_ID = "access-workspace-fill-panel";
  const PANEL_STYLE_ID = "access-workspace-fill-panel-style";
  const PANEL_MODE_FILL = "fill";
  const PANEL_MODE_SAVE = "save";
  let scheduled = false;
  let savePanelDraft = null;
  let fillPanelDraft = null;
  let recentAutofill = null;
  let dismissedURL = "";
  let observedURL = "";

  function matchLabel(match) {
    return `${match.resourceName}${match.username ? ` (${match.username})` : ""}${match.personal ? " [personal]" : " [shared]"}`;
  }

  // Hash-less page key so SPA fragment navigation does not resurrect a dismissed panel.
  function currentPageKey() {
    return window.location.origin + window.location.pathname + window.location.search;
  }

  function syncNavigationState() {
    if (observedURL !== currentPageKey()) {
      observedURL = currentPageKey();
      dismissedURL = "";
    }
  }

  function dismissSuggestions() {
    dismissedURL = currentPageKey();
    removePanel();
    // Tell the background so panels in every frame of this tab close too.
    void sendMessage({ type: "dismiss-portal-suggestions", url: window.location.href }).catch(() => null);
  }

  // Closes the credential pick list of the fill panel (not the panel itself).
  // Returns true when an open list was closed.
  function closeFillPickerMenu() {
    const panel = document.getElementById(PANEL_ID);
    if (!panel || panel.dataset.mode !== PANEL_MODE_FILL) {
      return false;
    }
    const menu = panel.querySelector(".aw-fill-picker-menu");
    if (!(menu instanceof HTMLElement) || menu.hidden) {
      return false;
    }
    menu.hidden = true;
    const picker = panel.querySelector(".aw-fill-picker");
    if (picker instanceof HTMLElement) {
      picker.dataset.open = "false";
    }
    const toggle = panel.querySelector(".aw-fill-picker-toggle");
    if (toggle instanceof HTMLElement) {
      toggle.textContent = "Select";
    }
    return true;
  }

  function isSupportedPage() {
    return window.location.protocol === "http:" || window.location.protocol === "https:";
  }

  function isTopLevelFrame() {
    return window.self === window.top;
  }

  function visible(element) {
    if (!(element instanceof HTMLElement)) {
      return false;
    }
    const style = window.getComputedStyle(element);
    const rect = element.getBoundingClientRect();
    return style.display !== "none" && style.visibility !== "hidden" && rect.width > 0 && rect.height > 0;
  }

  function firstVisiblePasswordField() {
    return Array.from(document.querySelectorAll('input[type="password"]')).find(visible);
  }

  function candidateUsernameFields() {
    return Array.from(document.querySelectorAll('input'))
      .filter((input) => {
        if (!(input instanceof HTMLInputElement) || !visible(input)) {
          return false;
        }
        const type = (input.type || "text").toLowerCase();
        if (!["text", "email", "search", "tel", "url"].includes(type)) {
          return false;
        }
        const hint = `${input.name} ${input.id} ${input.autocomplete} ${input.placeholder}`.toLowerCase();
        return /(user|email|login|account)/.test(hint) || type === "email";
      });
  }

  function firstVisibleUsernameField() {
    return candidateUsernameFields()[0] || null;
  }

  function firstFilledInput(form, selectorList) {
    for (const selector of selectorList) {
      const match = Array.from(form.querySelectorAll(selector)).find((element) => {
        return element instanceof HTMLInputElement && String(element.value || "").trim() !== "";
      });
      if (match instanceof HTMLInputElement) {
        return match;
      }
    }
    return null;
  }

  function dispatchValue(input, value) {
    input.focus();
    input.value = value;
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }

  function setRecentAutofill(candidate) {
    recentAutofill = {
      username: String(candidate?.username || "").trim(),
      password: String(candidate?.password || ""),
      createdAt: Date.now()
    };
  }

  function matchesRecentAutofill(candidate) {
    if (!recentAutofill) {
      return false;
    }
    if (Date.now() - recentAutofill.createdAt > 60 * 1000) {
      recentAutofill = null;
      return false;
    }
    return recentAutofill.username === String(candidate?.username || "").trim() && recentAutofill.password === String(candidate?.password || "");
  }

  function submitLoginForm(field) {
    if (!(field instanceof HTMLElement)) {
      return false;
    }
    const form = field.closest("form");
    if (form instanceof HTMLFormElement) {
      if (typeof form.requestSubmit === "function") {
        form.requestSubmit();
        return true;
      }
      form.submit();
      return true;
    }
    const container = field.closest('[role="form"], [data-testid*="login" i], [class*="login" i], [class*="sign-in" i]');
    const submitButton = (container || document).querySelector(
      'button[type="submit"], input[type="submit"], button[name*="login" i], button[id*="login" i], button[class*="login" i], button[name*="sign" i], button[id*="sign" i], button[class*="sign" i]'
    );
    if (submitButton instanceof HTMLElement) {
      submitButton.click();
      return true;
    }
    return false;
  }

  function chooseUsernameField(passwordField) {
    const candidates = candidateUsernameFields();
    if (candidates.length === 0) {
      return null;
    }
    const passwordTop = passwordField.getBoundingClientRect().top;
    return (
      candidates
        .filter((candidate) => candidate.getBoundingClientRect().top <= passwordTop + 40)
        .sort((left, right) => Math.abs(left.getBoundingClientRect().top - passwordTop) - Math.abs(right.getBoundingClientRect().top - passwordTop))[0] ||
      candidates[0]
    );
  }

  function removePanel(preserveFillDraft = false) {
    if (!preserveFillDraft) {
      fillPanelDraft = null;
    }
    document.getElementById(PANEL_ID)?.remove();
  }

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

  async function injectPanel(matches) {
    // In small embedded frames the panel cannot be shown fully (its close
    // button ends up clipped and it covers the page underneath), so skip them.
    if (!isTopLevelFrame() && (window.innerWidth < 260 || window.innerHeight < 200)) {
      removePanel();
      return;
    }
    const passwordField = firstVisiblePasswordField();
    const usernameField = firstVisibleUsernameField();
    if ((!passwordField && !usernameField) || matches.length === 0) {
      removePanel();
      return;
    }

    const panel = ensurePanel();
    panel.dataset.mode = PANEL_MODE_FILL;
    panel.innerHTML = `
        <div class="aw-fill-card">
          <div class="aw-fill-header">
            <strong>Access Workspace</strong>
            <button type="button" class="aw-fill-close" aria-label="Dismiss">x</button>
          </div>
          <p class="aw-fill-copy">Suggested portal credential for this page.</p>
          <div class="aw-fill-picker">
            <button type="button" class="aw-fill-picker-trigger">
              <span class="aw-fill-picker-value"></span>
              <span class="aw-fill-picker-toggle">Select</span>
            </button>
            <div class="aw-fill-picker-menu" hidden></div>
          </div>
          <button type="button" class="aw-fill-action">Use credential</button>
          <p class="aw-fill-status" aria-live="polite"></p>
        </div>
      `;

    const picker = panel.querySelector(".aw-fill-picker");
    const pickerTrigger = panel.querySelector(".aw-fill-picker-trigger");
    const pickerValue = panel.querySelector(".aw-fill-picker-value");
    const pickerToggle = panel.querySelector(".aw-fill-picker-toggle");
    const pickerMenu = panel.querySelector(".aw-fill-picker-menu");
    const action = panel.querySelector(".aw-fill-action");
    const status = panel.querySelector(".aw-fill-status");
    const close = panel.querySelector(".aw-fill-close");
    if (
      !(picker instanceof HTMLElement) ||
      !(pickerTrigger instanceof HTMLButtonElement) ||
      !(pickerValue instanceof HTMLElement) ||
      !(pickerToggle instanceof HTMLElement) ||
      !(pickerMenu instanceof HTMLElement) ||
      !(action instanceof HTMLButtonElement) ||
      !(status instanceof HTMLElement) ||
      !(close instanceof HTMLButtonElement)
    ) {
      return;
    }

    const selectedResourceId =
      fillPanelDraft?.selectedResourceId && matches.some((match) => match.resourceId === fillPanelDraft.selectedResourceId)
        ? fillPanelDraft.selectedResourceId
        : matches[0]?.resourceId || "";
    const selectedMatch = matches.find((match) => match.resourceId === selectedResourceId) || matches[0];

    const closePicker = () => {
      pickerMenu.hidden = true;
      picker.dataset.open = "false";
      pickerToggle.textContent = "Select";
    };

    const setSelectedResource = (resourceId) => {
      const nextSelected = matches.find((match) => match.resourceId === resourceId) || matches[0];
      if (!nextSelected) {
        return;
      }
      panel.dataset.selectedResourceId = nextSelected.resourceId;
      pickerValue.textContent = matchLabel(nextSelected);
      fillPanelDraft = {
        selectedResourceId: nextSelected.resourceId
      };
      Array.from(pickerMenu.querySelectorAll(".aw-fill-picker-option")).forEach((element) => {
        if (!(element instanceof HTMLButtonElement)) {
          return;
        }
        element.classList.toggle("active", element.dataset.resourceId === nextSelected.resourceId);
      });
    };

    pickerMenu.innerHTML = "";
    for (const match of matches) {
      const option = document.createElement("button");
      option.type = "button";
      option.className = "aw-fill-picker-option";
      option.dataset.resourceId = match.resourceId;
      const title = document.createElement("strong");
      title.textContent = match.resourceName;
      const subtitle = document.createElement("span");
      subtitle.textContent = `${match.username || ""}${match.personal ? " personal password" : " shared password"}`;
      option.appendChild(title);
      option.appendChild(subtitle);
      option.onclick = () => {
        setSelectedResource(match.resourceId);
        closePicker();
      };
      pickerMenu.appendChild(option);
    }

    close.onclick = () => {
      closePicker();
      dismissSuggestions();
    };
    if (matches.length < 2) {
      // Only one credential matches — there is nothing to pick.
      pickerTrigger.disabled = true;
      pickerToggle.textContent = "";
    } else {
      pickerTrigger.onclick = () => {
        const nextOpen = picker.dataset.open !== "true";
        picker.dataset.open = nextOpen ? "true" : "false";
        pickerMenu.hidden = !nextOpen;
        pickerToggle.textContent = nextOpen ? "Close" : "Select";
      };
    }
    setSelectedResource(selectedMatch?.resourceId || "");
    action.textContent = passwordField ? "Use credential" : "Fill username";
    status.textContent = passwordField
      ? "Username and password will be filled where the page allows it."
      : "This page looks like the first login step, so the extension will fill only the username/email.";
    action.onclick = async () => {
      action.disabled = true;
      status.textContent = passwordField ? "Filling credential..." : "Filling username...";
      try {
        const chosenResourceId = panel.dataset.selectedResourceId || fillPanelDraft?.selectedResourceId || "";
        const fill = await sendMessage({
          type: "portal-fill",
          resourceId: chosenResourceId,
          url: window.location.href
        });
        const currentPasswordField = firstVisiblePasswordField();
        const currentUsernameField = firstVisibleUsernameField();
        if (!(currentPasswordField instanceof HTMLInputElement) && !(currentUsernameField instanceof HTMLInputElement)) {
          throw new Error("No visible login field found on this page.");
        }
        if (currentPasswordField instanceof HTMLInputElement) {
          const nearestUsernameField = chooseUsernameField(currentPasswordField) ?? currentUsernameField;
          if (nearestUsernameField instanceof HTMLInputElement && fill.username) {
            dispatchValue(nearestUsernameField, fill.username);
          }
          dispatchValue(currentPasswordField, fill.password || "");
          setRecentAutofill(fill);
          const submitted = submitLoginForm(currentPasswordField) || submitLoginForm(nearestUsernameField);
          status.textContent = submitted ? `Filled ${fill.resourceName} and submitted the sign-in form.` : `Filled ${fill.resourceName}.`;
          return;
        }
        if (currentUsernameField instanceof HTMLInputElement && fill.username) {
          dispatchValue(currentUsernameField, fill.username);
          setRecentAutofill({
            username: fill.username,
            password: ""
          });
          const submitted = submitLoginForm(currentUsernameField);
          removePanel(true);
          status.textContent = submitted
            ? `Filled username for ${fill.resourceName} and submitted the first login step.`
            : `Filled username for ${fill.resourceName}. Continue to the password step and the suggestion should appear there too.`;
          return;
        }
        throw new Error("No compatible username field was found on this page.");
      } catch (error) {
        status.textContent = error instanceof Error ? error.message : "Credential fill failed.";
      } finally {
        action.disabled = false;
      }
    };
  }

  function ensurePanel() {
    ensurePanelStyle();
    let panel = document.getElementById(PANEL_ID);
    if (panel) {
      return panel;
    }
    panel = document.createElement("div");
    panel.id = PANEL_ID;
    document.body.appendChild(panel);
    return panel;
  }

  function ensurePanelStyle() {
    if (document.getElementById(PANEL_STYLE_ID)) {
      return;
    }
    const style = document.createElement("style");
    style.id = PANEL_STYLE_ID;
    style.textContent = `
      #${PANEL_ID} {
        position: fixed;
        right: 16px;
        top: 16px;
        z-index: 2147483647;
        font-family: ui-sans-serif, system-ui, sans-serif;
      }
      #${PANEL_ID} .aw-fill-card {
        width: 320px;
        border: 1px solid #29415c;
        background: #0e1826;
        color: #edf4ff;
        border-radius: 16px;
        box-shadow: 0 24px 60px rgba(0, 0, 0, 0.35);
        padding: 14px;
      }
      @media (max-width: 900px) {
        #${PANEL_ID} {
          top: auto;
          right: 12px;
          bottom: 12px;
          left: 12px;
        }
        #${PANEL_ID} .aw-fill-card {
          width: auto;
        }
      }
      #${PANEL_ID} .aw-fill-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        gap: 12px;
        margin-bottom: 8px;
      }
      #${PANEL_ID} .aw-fill-copy,
      #${PANEL_ID} .aw-fill-status,
      #${PANEL_ID} .aw-fill-note {
        margin: 0 0 10px;
        font-size: 13px;
        line-height: 1.4;
        color: #c6d6eb;
      }
      #${PANEL_ID} .aw-fill-picker-trigger,
      #${PANEL_ID} .aw-fill-picker-option,
      #${PANEL_ID} .aw-fill-action,
      #${PANEL_ID} .aw-save-input {
        width: 100%;
        box-sizing: border-box;
        border-radius: 12px;
        border: 1px solid #3b5775;
        padding: 10px 12px;
        font: inherit;
      }
      #${PANEL_ID} .aw-fill-picker,
      #${PANEL_ID} .aw-fill-picker-trigger,
      #${PANEL_ID} .aw-save-input {
        margin-bottom: 10px;
      }
      #${PANEL_ID} .aw-fill-picker-trigger,
      #${PANEL_ID} .aw-save-input {
        background: #152334;
        color: #edf4ff;
        text-align: left;
      }
      #${PANEL_ID} .aw-fill-picker {
        position: relative;
      }
      #${PANEL_ID} .aw-fill-picker-trigger {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        cursor: pointer;
      }
      #${PANEL_ID} .aw-fill-picker-trigger:disabled {
        cursor: default;
      }
      #${PANEL_ID} .aw-fill-picker-value {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
      #${PANEL_ID} .aw-fill-picker-toggle {
        flex: none;
        color: #c6d6eb;
        font-size: 12px;
      }
      #${PANEL_ID} .aw-fill-picker-menu {
        position: absolute;
        top: calc(100% - 2px);
        left: 0;
        right: 0;
        z-index: 2;
        display: grid;
        gap: 8px;
        padding: 10px;
        border: 1px solid #3b5775;
        border-radius: 14px;
        background: rgba(9, 21, 35, 0.98);
        box-shadow: 0 18px 40px rgba(0, 0, 0, 0.28);
        max-height: 220px;
        overflow-y: auto;
        overscroll-behavior: contain;
      }
      /* Our display:grid above would defeat the hidden attribute on pages
         without a [hidden] reset of their own, leaving the list stuck open. */
      #${PANEL_ID} .aw-fill-picker-menu[hidden] {
        display: none !important;
      }
      #${PANEL_ID} .aw-fill-picker-option {
        display: grid;
        gap: 4px;
        background: rgba(255, 255, 255, 0.03);
        color: #edf4ff;
        text-align: left;
        cursor: pointer;
      }
      #${PANEL_ID} .aw-fill-picker-option span {
        color: #c6d6eb;
        font-size: 12px;
      }
      #${PANEL_ID} .aw-fill-picker-option.active {
        border-color: rgba(159, 204, 255, 0.36);
        background: rgba(159, 204, 255, 0.1);
      }
      #${PANEL_ID} .aw-fill-action {
        background: linear-gradient(135deg, #ffb86a, #ffd39d);
        color: #201000;
        cursor: pointer;
        font-weight: 600;
      }
      #${PANEL_ID} .aw-fill-close {
        border: 0;
        background: transparent;
        color: #c6d6eb;
        cursor: pointer;
        font: inherit;
      }
      #${PANEL_ID} .aw-save-checkbox {
        display: flex;
        align-items: center;
        gap: 8px;
        margin: 0 0 10px;
        font-size: 13px;
        color: #edf4ff;
      }
      #${PANEL_ID} .aw-save-actions {
        display: flex;
        gap: 8px;
      }
      #${PANEL_ID} .aw-save-cancel {
        flex: 1;
        border-radius: 12px;
        border: 1px solid #3b5775;
        background: #152334;
        color: #edf4ff;
        cursor: pointer;
        padding: 10px 12px;
        font: inherit;
      }
    `;
    document.head.appendChild(style);
  }

  async function injectSavePanel(candidate) {
    if (!candidate) {
      return;
    }
    const isUpdate = candidate.mode === "update";
    const draft = {
      name: savePanelDraft?.name ?? candidate.name ?? "",
      url: savePanelDraft?.url ?? candidate.url ?? "",
      username: savePanelDraft?.username ?? candidate.username ?? "",
      password: savePanelDraft?.password ?? candidate.password ?? "",
      personal: savePanelDraft?.personal ?? true
    };
    const panel = ensurePanel();
    panel.dataset.mode = PANEL_MODE_SAVE;
    panel.replaceChildren();

    const card = document.createElement("div");
    card.className = "aw-fill-card";

    const header = document.createElement("div");
    header.className = "aw-fill-header";

    const title = document.createElement("strong");
    title.textContent = "Access Workspace";

    const closeButton = document.createElement("button");
    closeButton.type = "button";
    closeButton.className = "aw-fill-close";
    closeButton.setAttribute("aria-label", "Dismiss");
    closeButton.textContent = "x";

    header.append(title, closeButton);

    const copy = document.createElement("p");
    copy.className = "aw-fill-copy";
    copy.textContent = isUpdate ? "Update the stored password for this portal credential." : "Save the credential you just used for this portal.";

    const nameInput = document.createElement("input");
    nameInput.className = "aw-save-input aw-save-name";
    nameInput.placeholder = "Name";

    const urlInput = document.createElement("input");
    urlInput.className = "aw-save-input aw-save-url";
    urlInput.placeholder = "Portal URL";

    const usernameInput = document.createElement("input");
    usernameInput.className = "aw-save-input aw-save-username";
    usernameInput.placeholder = "Username or email";

    const passwordInput = document.createElement("input");
    passwordInput.className = "aw-save-input aw-save-password";
    passwordInput.type = "password";
    passwordInput.placeholder = "Password";

    const personalLabel = document.createElement("label");
    personalLabel.className = "aw-save-checkbox";

    const personalInput = document.createElement("input");
    personalInput.className = "aw-save-personal";
    personalInput.type = "checkbox";
    personalInput.checked = draft.personal;
    personalInput.disabled = isUpdate || !candidate.canSaveShared;

    const personalText = document.createElement("span");
    personalText.textContent = "Save as personal password";
    personalLabel.append(personalInput, personalText);

    const noteText = document.createElement("p");
    noteText.className = "aw-fill-note";

    const actions = document.createElement("div");
    actions.className = "aw-save-actions";

    const submitButton = document.createElement("button");
    submitButton.type = "button";
    submitButton.className = "aw-fill-action aw-save-submit";
    submitButton.textContent = isUpdate ? "Update password" : "Save credential";

    const cancelButton = document.createElement("button");
    cancelButton.type = "button";
    cancelButton.className = "aw-save-cancel";
    cancelButton.textContent = "Dismiss";

    actions.append(submitButton, cancelButton);

    const statusText = document.createElement("p");
    statusText.className = "aw-fill-status";
    statusText.setAttribute("aria-live", "polite");

    card.append(
      header,
      copy,
      nameInput,
      urlInput,
      usernameInput,
      passwordInput,
      personalLabel,
      noteText,
      actions,
      statusText
    );
    panel.appendChild(card);

    const close = panel.querySelector(".aw-fill-close");
    const cancel = panel.querySelector(".aw-save-cancel");
    const submit = panel.querySelector(".aw-save-submit");
    const status = panel.querySelector(".aw-fill-status");
    const personal = panel.querySelector(".aw-save-personal");
    const name = panel.querySelector(".aw-save-name");
    const url = panel.querySelector(".aw-save-url");
    const username = panel.querySelector(".aw-save-username");
    const password = panel.querySelector(".aw-save-password");
    const note = panel.querySelector(".aw-fill-note");
    if (!(close instanceof HTMLButtonElement) ||
        !(cancel instanceof HTMLButtonElement) ||
        !(submit instanceof HTMLButtonElement) ||
        !(status instanceof HTMLElement) ||
        !(personal instanceof HTMLInputElement) ||
        !(name instanceof HTMLInputElement) ||
        !(url instanceof HTMLInputElement) ||
        !(username instanceof HTMLInputElement) ||
        !(password instanceof HTMLInputElement) ||
        !(note instanceof HTMLElement)) {
      return;
    }

    name.value = draft.name;
    url.value = draft.url;
    username.value = draft.username;
    password.value = draft.password;
    note.textContent = isUpdate
      ? `Matched ${candidate.existingResourceName || "an existing credential"} by portal URL and username.`
      : candidate.canSaveShared
        ? "Uncheck this only when you intentionally want to save a shared portal credential."
        : "Shared save is admin-only in this first extension flow, so this will be saved as personal.";

    const dismiss = async () => {
      savePanelDraft = null;
      await sendMessage({ type: "clear-pending-save" }).catch(() => null);
      removePanel();
    };

    const syncDraft = () => {
      savePanelDraft = {
        name: name.value,
        url: url.value,
        username: username.value,
        password: password.value,
        personal: personal.checked
      };
    };

    name.addEventListener("input", syncDraft);
    url.addEventListener("input", syncDraft);
    username.addEventListener("input", syncDraft);
    password.addEventListener("input", syncDraft);
    personal.addEventListener("change", syncDraft);
    syncDraft();

    close.onclick = () => void dismiss();
    cancel.onclick = () => void dismiss();
    submit.onclick = async () => {
      submit.disabled = true;
      status.textContent = isUpdate ? "Updating stored password..." : "Saving credential...";
      try {
        const result = await sendMessage({
          type: "save-portal-credential",
          input: {
            name: name.value,
            url: url.value,
            username: username.value,
            password: password.value,
            personal: candidate.canSaveShared ? personal.checked : true
          }
        });
        savePanelDraft = null;
        status.textContent = result.action === "updated"
          ? `Updated password for ${result.name}.`
          : `Saved ${result.name} as ${result.personal ? "personal" : "shared"}.`;
        window.setTimeout(() => removePanel(), 1200);
      } catch (error) {
        status.textContent = error instanceof Error ? error.message : isUpdate ? "Updating the credential failed." : "Saving the credential failed.";
      } finally {
        submit.disabled = false;
      }
    };
  }

  function loginCandidateFromForm(form) {
    if (!(form instanceof HTMLFormElement)) {
      return null;
    }
    const passwordField = firstFilledInput(form, ['input[type="password"]']);
    const usernameField = firstFilledInput(form, [
      'input[type="email"]',
      'input[autocomplete="username"]',
      'input[name*="user" i]',
      'input[name*="email" i]',
      'input[id*="user" i]',
      'input[id*="email" i]',
      'input[type="text"]'
    ]);
    if (!(passwordField instanceof HTMLInputElement) || !(usernameField instanceof HTMLInputElement)) {
      return null;
    }
    const username = String(usernameField.value || "").trim();
    const password = String(passwordField.value || "");
    if (!username || !password) {
      return null;
    }
    return {
      name: document.title || window.location.hostname,
      title: document.title || "",
      url: window.location.href,
      username,
      password
    };
  }

  async function refreshSuggestions() {
    scheduled = false;
    syncNavigationState();
    if (!isSupportedPage()) {
      removePanel();
      return;
    }
    if (dismissedURL === currentPageKey()) {
      removePanel();
      return;
    }
    try {
      const pendingSave = await sendMessage({
        type: "get-pending-save",
        url: window.location.href
      });
      if (pendingSave) {
        if (!isTopLevelFrame()) {
          removePanel();
          return;
        }
        if (document.getElementById(PANEL_ID)?.dataset.mode === PANEL_MODE_SAVE) {
          return;
        }
        await injectSavePanel(pendingSave);
        return;
      }
    } catch {
      // ignore save-prompt lookup failures and keep normal fill flow
    }
    if (!firstVisiblePasswordField() && !firstVisibleUsernameField()) {
      removePanel();
      return;
    }
    try {
      const matches = await sendMessage({
        type: "portal-matches",
        url: window.location.href
      });
      if (document.getElementById(PANEL_ID)?.dataset.mode === PANEL_MODE_FILL && fillPanelDraft?.selectedResourceId) {
        const normalizedMatches = Array.isArray(matches) ? matches : [];
        const selectedStillExists = normalizedMatches.some((match) => match.resourceId === fillPanelDraft.selectedResourceId);
        const currentSelectedResourceId = document.getElementById(PANEL_ID)?.dataset.selectedResourceId;
        if (selectedStillExists && currentSelectedResourceId === fillPanelDraft.selectedResourceId) {
          return;
        }
      }
      await injectPanel(Array.isArray(matches) ? matches : []);
    } catch {
      removePanel();
    }
  }

  function scheduleRefresh() {
    if (scheduled) {
      return;
    }
    scheduled = true;
    window.setTimeout(() => {
      void refreshSuggestions();
    }, 250);
  }

  const observer = new MutationObserver(() => scheduleRefresh());
  observer.observe(document.documentElement, { childList: true, subtree: true });
  window.addEventListener("message", (event) => {
    if (event.source !== window) {
      return;
    }
    const data = event.data;
    if (!data || data.source !== "access-workspace-webapp" || data.type !== "connect-browser-extension") {
      return;
    }
    void sendMessage({
      type: "connect-workspace",
      workspaceBaseUrl: data.payload?.workspaceBaseUrl,
      connectToken: data.payload?.connectToken
    })
      .then((result) => {
        window.postMessage({
          source: "access-workspace-extension",
          requestId: data.requestId,
          ok: true,
          result
        }, window.location.origin);
      })
      .catch((error) => {
        window.postMessage({
          source: "access-workspace-extension",
          requestId: data.requestId,
          ok: false,
          error: error instanceof Error ? error.message : "Connecting the extension failed."
        }, window.location.origin);
      });
  });
  document.addEventListener("submit", (event) => {
    const candidate = loginCandidateFromForm(event.target);
    if (!candidate) {
      return;
    }
    if (matchesRecentAutofill(candidate)) {
      return;
    }
    void sendMessage({
      type: "report-save-candidate",
      candidate
    }).catch(() => null);
  }, true);
  document.addEventListener("pointerdown", (event) => {
    const panel = document.getElementById(PANEL_ID);
    if (!panel || panel.dataset.mode !== PANEL_MODE_FILL) {
      return;
    }
    const picker = panel.querySelector(".aw-fill-picker");
    if (picker instanceof HTMLElement && event.target instanceof Node && picker.contains(event.target)) {
      return;
    }
    closeFillPickerMenu();
  }, true);
  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") {
      return;
    }
    const panel = document.getElementById(PANEL_ID);
    if (!panel || panel.dataset.mode !== PANEL_MODE_FILL) {
      return;
    }
    // First Escape closes the open pick list, the next one dismisses the panel.
    if (closeFillPickerMenu()) {
      return;
    }
    dismissSuggestions();
  }, true);
  window.addEventListener("focus", scheduleRefresh);
  window.setInterval(scheduleRefresh, 4000);
  scheduleRefresh();
})();

function escapeHtml(value) {
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
