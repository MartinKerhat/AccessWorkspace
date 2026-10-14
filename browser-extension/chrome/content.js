(function contentScript() {
  const PANEL_ID = "access-workspace-fill-panel";
  const PANEL_STYLE_ID = "access-workspace-fill-panel-style";
  const PANEL_MODE_FILL = "fill";
  const PANEL_MODE_SAVE = "save";
  const PANEL_MODE_GENERATE = "generate";
  let scheduled = false;
  let savePanelDraft = null;
  let fillPanelDraft = null;
  // Sign-up generation state: the chip's own options and the last value the
  // user accepted (so the submit capture can mark the candidate as generated).
  let generatePanelDraft = null;
  let lastGenerated = "";
  // Closing the chip hides only the chip, on this page; it never touches the
  // pending save (a generated password must always be offered for saving).
  let generateDismissedKey = "";

  const SUBMIT_TEXT_HINT = /(register|sign ?up|create|continue|next|save|submit|confirm|finish|done|pokra|zaregistr|vytvo|ulo|zalo|dokon|potvrd|odesl)/i;

  // A generated password is on the page and the user is pressing something
  // that looks like it submits the form — real <form> submit or not.
  function reportGeneratedOnSubmitIntent(target) {
    if (!lastGenerated || !(target instanceof Element)) {
      return;
    }
    const button = target.closest('button, input[type="submit"], input[type="button"], [role="button"]');
    if (!(button instanceof HTMLElement)) {
      return;
    }
    const text = `${button.textContent || ""} ${button.getAttribute("value") || ""} ${button.getAttribute("aria-label") || ""}`.trim();
    const isSubmit = (button instanceof HTMLButtonElement && button.type === "submit") || (button instanceof HTMLInputElement && button.type === "submit");
    if (!isSubmit && !SUBMIT_TEXT_HINT.test(text)) {
      return;
    }
    const stillOnPage = Array.from(document.querySelectorAll('input[type="password"]')).some((field) => field instanceof HTMLInputElement && field.value === lastGenerated);
    if (!stillOnPage) {
      return;
    }
    const usernameField = loginFieldForPassword(generatedPasswordField(), true);
    void sendMessage({
      type: "report-save-candidate",
      candidate: {
        name: document.title || window.location.hostname,
        title: document.title || "",
        url: window.location.href,
        username: usernameField instanceof HTMLInputElement ? String(usernameField.value || "").trim() : "",
        password: lastGenerated,
        generated: true
      }
    }).catch(() => null);
  }
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
      generateDismissedKey = "";
      if (lastGenerated) {
        // The page moved on after a generated password was used: whatever
        // submitted it, the save card must come up now.
        void sendMessage({ type: "lift-pending-save-deferral" }).catch(() => null);
      }
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

  // ---- the workspace itself is never a target ----
  // Two signals, no configured URL: the web app marks its own HTML with
  // <meta name="access-workspace-app">, and the origin the extension was
  // connected from is remembered (webAppOrigin). Either one silences every
  // panel here; the connect handshake listener keeps working regardless.
  let workspaceOriginKnown = null;

  function isWorkspaceAppPage() {
    if (document.querySelector('meta[name="access-workspace-app"]')) {
      return true;
    }
    return workspaceOriginKnown === window.location.origin;
  }

  async function refreshWorkspaceOrigin() {
    try {
      const config = await sendMessage({ type: "get-config" });
      workspaceOriginKnown = String(config?.webAppOrigin || "");
    } catch {
      workspaceOriginKnown = null;
    }
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

  // ---- password context: login vs sign-up / change ----
  // The fill panel belongs to login forms only; the generate chip to forms
  // that ask for a NEW password. Signals, in order of trust: autocomplete
  // tokens, then two password inputs in one form, then name/id/label hints.
  const NEW_PASSWORD_HINT = /(new|confirm|repeat|retype|verify|create|register|signup|sign-up|sign_up)/i;
  const CURRENT_PASSWORD_HINT = /(current|old|existing)/i;

  function fieldHint(input) {
    const labels = input.labels && input.labels.length ? Array.from(input.labels).map((label) => label.textContent || "").join(" ") : "";
    return `${input.name} ${input.id} ${input.autocomplete} ${input.placeholder} ${input.getAttribute("aria-label") || ""} ${labels}`;
  }

  function isCurrentPasswordField(input) {
    const autocomplete = String(input.autocomplete || "").toLowerCase();
    if (autocomplete === "current-password") {
      return true;
    }
    if (autocomplete === "new-password") {
      return false;
    }
    return CURRENT_PASSWORD_HINT.test(fieldHint(input));
  }

  function isNewPasswordField(input) {
    const autocomplete = String(input.autocomplete || "").toLowerCase();
    if (autocomplete === "new-password") {
      return true;
    }
    if (autocomplete === "current-password") {
      return false;
    }
    const hint = fieldHint(input);
    return NEW_PASSWORD_HINT.test(hint) && !CURRENT_PASSWORD_HINT.test(hint);
  }

  function passwordContext() {
    const fields = Array.from(document.querySelectorAll('input[type="password"]')).filter(visible);
    if (fields.length === 0) {
      return { kind: "none", newFields: [], currentField: null };
    }
    const form = fields[0].closest("form");
    const inForm = form ? fields.filter((field) => field.closest("form") === form) : fields;
    const current = inForm.filter(isCurrentPasswordField);
    const explicitNew = inForm.filter(isNewPasswordField);
    const others = inForm.filter((field) => !current.includes(field));
    let newFields = [];
    if (inForm.length >= 2 && others.length > 0 && (explicitNew.length > 0 || current.length < inForm.length)) {
      // Two or more password boxes: every box that is not the current
      // password is a new-password box (new + confirm), even when only one
      // of them carries an explicit hint — a form labelled "Password" /
      // "Confirm password" must get both filled.
      newFields = others;
    } else if (explicitNew.length > 0) {
      newFields = explicitNew;
    }
    if (newFields.length === 0) {
      return { kind: "login", newFields: [], currentField: inForm[0] };
    }
    return { kind: current.length > 0 ? "change" : "signup", newFields, currentField: current[0] || null };
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

  // ---- which field is the LOGIN on a sign-up form ----
  // Page order is a poor guide: an e-mail box often sits far above the real
  // login name. Rank by what the field says it is, then by closeness to the
  // password box; at save time only boxes that actually hold a value count.
  const LOGIN_HINT = /(login|user ?name|username|account ?name|screen ?name|nick|p[rř]ihla[sš]|u[zž]ivatelsk[eé] jm|\buser\b|\buid\b)/i;
  const EMAIL_HINT = /(e-?mail)/i;

  function labelsText(input) {
    const own = input.labels && input.labels.length ? Array.from(input.labels).map((label) => label.textContent || "").join(" ") : "";
    // Table layouts put the caption in the preceding cell without a <label for>.
    const cell = input.closest("td, th");
    const caption = cell && cell.previousElementSibling ? cell.previousElementSibling.textContent || "" : "";
    const wrapper = input.closest("label");
    const wrapped = wrapper ? wrapper.textContent || "" : "";
    return `${own} ${caption.length < 80 ? caption : ""} ${wrapped.length < 80 ? wrapped : ""}`;
  }

  function loginFieldScore(input) {
    const autocomplete = String(input.autocomplete || "").toLowerCase();
    if (autocomplete === "username") {
      return 3;
    }
    const identity = `${input.name} ${input.id} ${input.placeholder} ${input.getAttribute("aria-label") || ""} ${labelsText(input)}`;
    const type = String(input.type || "text").toLowerCase();
    const looksEmail = autocomplete === "email" || type === "email" || EMAIL_HINT.test(identity);
    const looksLogin = LOGIN_HINT.test(identity);
    if (looksLogin && !looksEmail) {
      return 2;
    }
    if (looksEmail) {
      return 1;
    }
    return 0;
  }

  // The login box for the given password box: highest score wins, ties go to
  // the nearest box above the password. With requireValue only filled boxes
  // qualify. Returns null when nothing plausible exists.
  function loginFieldForPassword(passwordField, requireValue) {
    const anchor = passwordField instanceof HTMLInputElement ? passwordField : firstVisiblePasswordField();
    const form = anchor ? anchor.closest("form") : null;
    const scope = form || document;
    const anchorTop = anchor ? anchor.getBoundingClientRect().top : Number.POSITIVE_INFINITY;
    const candidates = Array.from(scope.querySelectorAll("input"))
      .filter((input) => {
        if (!(input instanceof HTMLInputElement) || !visible(input) || input.disabled || input.readOnly) {
          return false;
        }
        const type = String(input.type || "text").toLowerCase();
        if (!["text", "email", "tel", "url", "search"].includes(type)) {
          return false;
        }
        return !requireValue || String(input.value || "").trim() !== "";
      })
      .map((input) => {
        const top = input.getBoundingClientRect().top;
        return { input, score: loginFieldScore(input), distance: Math.abs(anchorTop - top), above: top <= anchorTop + 4 };
      })
      .filter((item) => item.score > 0 || (item.above && item.distance < 160));
    if (candidates.length === 0) {
      return null;
    }
    candidates.sort((a, b) => {
      if (b.score !== a.score) {
        return b.score - a.score;
      }
      if (a.above !== b.above) {
        return a.above ? -1 : 1;
      }
      return a.distance - b.distance;
    });
    return candidates[0].input;
  }

  function generatedPasswordField() {
    return (
      Array.from(document.querySelectorAll('input[type="password"]')).find((field) => field instanceof HTMLInputElement && field.value === lastGenerated) ||
      null
    );
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

  // ---- generate chip (sign-up / password-change forms) ----
  function element(tag, className, text) {
    const node = document.createElement(tag);
    if (className) {
      node.className = className;
    }
    if (text !== undefined) {
      node.textContent = text;
    }
    return node;
  }

  async function injectGeneratePanel(context) {
    if (!isTopLevelFrame() && (window.innerWidth < 260 || window.innerHeight < 200)) {
      removePanel();
      return;
    }
    const existing = document.getElementById(PANEL_ID);
    if (existing?.dataset.mode === PANEL_MODE_GENERATE && generatePanelDraft) {
      // Already shown; keep the user's current candidate and options.
      return;
    }
    let suggestion = null;
    try {
      suggestion = await sendMessage({ type: "generate-password", overrides: {} });
    } catch {
      suggestion = null;
    }
    if (!suggestion?.allowed) {
      if (existing?.dataset.mode === PANEL_MODE_GENERATE) {
        removePanel();
      }
      return;
    }
    const passphraseFirst = suggestion.kind === "passphrase";
    generatePanelDraft = {
      kind: passphraseFirst ? "passphrase" : "password",
      value: suggestion.value,
      password: passphraseFirst ? null : suggestion.options,
      passphrase: passphraseFirst ? suggestion.options : null,
      used: false
    };
    renderGeneratePanel(context);
  }

  // Current options of the chip as a settings document for the kind in use.
  function draftOverrides() {
    if (generatePanelDraft.kind === "passphrase") {
      return generatePanelDraft.passphrase ? { ...generatePanelDraft.passphrase } : {};
    }
    return generatePanelDraft.password ? { ...generatePanelDraft.password } : {};
  }

  function renderGeneratePanel(context) {
    const panel = ensurePanel();
    panel.dataset.mode = PANEL_MODE_GENERATE;
    panel.replaceChildren();
    const draft = generatePanelDraft;

    const card = element("div", "aw-fill-card");
    const header = element("div", "aw-fill-header");
    const title = element("strong", "", "Access Workspace");
    const closeButton = element("button", "aw-fill-close", "x");
    closeButton.type = "button";
    closeButton.setAttribute("aria-label", "Dismiss");
    header.append(title, closeButton);

    const copy = element(
      "p",
      "aw-fill-copy",
      context.kind === "change" ? "Use a strong new password for this change." : "Use a strong password for this sign-up."
    );

    // Password | Passphrase
    const kindRow = element("div", "aw-gen-segmented");
    const kindPassword = element("button", "aw-gen-kind", "Password");
    kindPassword.type = "button";
    const kindPassphrase = element("button", "aw-gen-kind", "Passphrase");
    kindPassphrase.type = "button";
    kindRow.append(kindPassword, kindPassphrase);

    // Length / words + no-symbols
    const optionsRow = element("div", "aw-gen-options");
    const numberLabel = element("label", "aw-gen-number-label");
    const numberText = element("span", "", "Length");
    const numberInput = document.createElement("input");
    numberInput.type = "number";
    numberInput.className = "aw-gen-number";
    numberLabel.append(numberText, numberInput);
    const toggleLabel = element("label", "aw-save-checkbox aw-gen-toggle");
    const toggleInput = document.createElement("input");
    toggleInput.type = "checkbox";
    toggleInput.className = "aw-gen-nosymbols";
    toggleLabel.append(toggleInput, element("span", "", "No symbols"));
    optionsRow.append(numberLabel, toggleLabel);

    const value = element("div", "aw-gen-value", draft.value);
    value.title = "Generated value";

    const actions = element("div", "aw-gen-row");
    const useButton = element("button", "aw-fill-action aw-gen-use", "Use");
    useButton.type = "button";
    const regenerateButton = element("button", "aw-save-cancel aw-gen-regenerate", "Regenerate");
    regenerateButton.type = "button";
    actions.append(useButton, regenerateButton);

    const saveRow = element("div", "aw-gen-row");
    const saveNowButton = element("button", "aw-save-cancel aw-gen-save-now", "Save now");
    saveNowButton.type = "button";
    saveRow.append(saveNowButton);
    saveRow.hidden = !draft.used;

    const status = element("p", "aw-fill-status", draft.used ? "Filled. The save card appears when you submit the form." : "");
    status.setAttribute("aria-live", "polite");

    card.append(header, copy, kindRow, optionsRow, value, actions, saveRow, status);
    panel.appendChild(card);

    const syncControls = () => {
      const passphrase = draft.kind === "passphrase";
      kindPassword.classList.toggle("active", !passphrase);
      kindPassphrase.classList.toggle("active", passphrase);
      numberText.textContent = passphrase ? "Words" : "Length";
      numberInput.min = passphrase ? "3" : "8";
      numberInput.max = passphrase ? "10" : "64";
      numberInput.value = String(passphrase ? draft.passphrase?.words ?? 5 : draft.password?.length ?? 20);
      toggleLabel.hidden = passphrase;
      toggleInput.checked = !passphrase && draft.password ? !draft.password.symbols : false;
      value.textContent = draft.value;
    };

    // Never disable the Regenerate button while generating: a click that
    // blurs the number box fires its change handler first, and a click on a
    // button disabled by that handler is swallowed. Re-entrancy is ignored.
    let regenerating = false;
    let lastAppliedNumber = null;
    const regenerate = async () => {
      if (regenerating) {
        return;
      }
      regenerating = true;
      try {
        const next = await sendMessage({ type: "generate-password", kind: draft.kind, overrides: draftOverrides() });
        if (next?.allowed && next.value) {
          draft.value = next.value;
          if (next.kind === "passphrase") {
            draft.passphrase = next.options;
          } else {
            draft.password = next.options;
          }
          syncControls();
        }
      } catch (error) {
        status.textContent = error instanceof Error ? error.message : "Generating failed.";
      } finally {
        regenerating = false;
      }
    };

    // Remember the chip's settings the way the web page does: when the
    // user acts on them (Regenerate / Use), not on every keystroke.
    const remember = () => {
      void sendMessage({
        type: "remember-generator-preference",
        part: draft.kind,
        settings: draftOverrides()
      }).catch(() => null);
    };

    syncControls();

    closeButton.onclick = () => {
      generatePanelDraft = null;
      generateDismissedKey = currentPageKey();
      removePanel();
    };
    const switchKind = (kind) => {
      if (draft.kind === kind) {
        return;
      }
      draft.kind = kind;
      lastAppliedNumber = null;
      void regenerate();
      // Same as switching on the Generator page: the choice is remembered.
      void sendMessage({ type: "remember-generator-preference", part: "passwords", settings: { kind } }).catch(() => null);
    };
    kindPassword.onclick = () => switchKind("password");
    kindPassphrase.onclick = () => switchKind("passphrase");
    const applyNumber = () => {
      const n = Math.floor(Number(numberInput.value));
      if (!Number.isFinite(n)) {
        syncControls();
        return;
      }
      if (n === lastAppliedNumber) {
        // Enter already applied it; the blur-triggered change is a repeat.
        return;
      }
      lastAppliedNumber = n;
      if (draft.kind === "passphrase") {
        draft.passphrase = { ...(draft.passphrase || {}), words: Math.min(10, Math.max(3, n)) };
      } else {
        draft.password = { ...(draft.password || {}), length: Math.min(64, Math.max(8, n)) };
      }
      void regenerate();
    };
    numberInput.onchange = applyNumber;
    numberInput.onkeydown = (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        applyNumber();
      }
    };
    toggleInput.onchange = () => {
      draft.password = { ...(draft.password || {}), symbols: !toggleInput.checked };
      void regenerate();
    };
    regenerateButton.onclick = () => {
      remember();
      void regenerate();
    };
    useButton.onclick = async () => {
      const current = passwordContext();
      const targets = current.newFields.length > 0 ? current.newFields : context.newFields.filter(visible);
      if (targets.length === 0) {
        status.textContent = "No password field found on this page any more.";
        return;
      }
      const password = draft.value;
      for (const field of targets) {
        dispatchValue(field, password);
      }
      lastGenerated = password;
      draft.used = true;
      remember();
      const usernameField = loginFieldForPassword(targets[0], true);
      void sendMessage({
        type: "report-save-candidate",
        candidate: {
          name: document.title || window.location.hostname,
          title: document.title || "",
          url: window.location.href,
          username: usernameField instanceof HTMLInputElement ? String(usernameField.value || "").trim() : "",
          password,
          generated: true,
          deferred: true
        }
      }).catch(() => null);
      saveRow.hidden = false;
      status.textContent = "Filled. The save card appears when you submit the form — or save it right away.";
    };
    saveNowButton.onclick = async () => {
      saveNowButton.disabled = true;
      try {
        const usernameField = loginFieldForPassword(generatedPasswordField() || passwordContext().newFields[0] || null, true);
        await sendMessage({
          type: "report-save-candidate",
          candidate: {
            name: document.title || window.location.hostname,
            title: document.title || "",
            url: window.location.href,
            username: usernameField instanceof HTMLInputElement ? String(usernameField.value || "").trim() : "",
            password: draft.value,
            generated: true
          }
        });
        const pending = await sendMessage({ type: "get-pending-save", url: window.location.href });
        if (pending) {
          generatePanelDraft = null;
          await injectSavePanel(pending);
          return;
        }
        status.textContent = "Could not open the save card. Submit the form and it will appear.";
      } catch (error) {
        status.textContent = error instanceof Error ? error.message : "Opening the save card failed.";
      } finally {
        saveNowButton.disabled = false;
      }
    };
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
      #${PANEL_ID} .aw-gen-value {
        margin: 0 0 10px;
        padding: 10px 12px;
        border: 1px solid #3b5775;
        border-radius: 12px;
        background: #152334;
        color: #edf4ff;
        font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
        font-size: 13px;
        word-break: break-all;
        user-select: all;
      }
      #${PANEL_ID} .aw-gen-row {
        display: flex;
        gap: 8px;
        margin-bottom: 10px;
      }
      #${PANEL_ID} .aw-gen-row .aw-fill-action {
        flex: 1;
      }
      #${PANEL_ID} .aw-gen-row[hidden] {
        display: none !important;
      }
      /* Pages hide native checkboxes for their own skins (opacity 0,
         zero size, off-screen); our controls must stay visible anyway. */
      #${PANEL_ID} input[type="checkbox"] {
        appearance: auto !important;
        -webkit-appearance: checkbox !important;
        opacity: 1 !important;
        position: static !important;
        width: 16px !important;
        height: 16px !important;
        margin: 0 !important;
        padding: 0 !important;
        pointer-events: auto !important;
        accent-color: #ffb86a;
      }
      #${PANEL_ID} .aw-gen-segmented {
        display: flex;
        gap: 4px;
        padding: 4px;
        margin: 0 0 10px;
        border: 1px solid #3b5775;
        border-radius: 12px;
        background: rgba(255, 255, 255, 0.04);
      }
      #${PANEL_ID} .aw-gen-kind {
        flex: 1;
        border: 0;
        border-radius: 9px;
        padding: 7px 10px;
        background: transparent;
        color: #c6d6eb;
        cursor: pointer;
        font: inherit;
        font-size: 13px;
      }
      #${PANEL_ID} .aw-gen-kind.active {
        background: rgba(159, 204, 255, 0.14);
        color: #edf4ff;
      }
      #${PANEL_ID} .aw-gen-options {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        margin: 0 0 10px;
      }
      #${PANEL_ID} .aw-gen-number-label {
        display: flex;
        align-items: center;
        gap: 8px;
        font-size: 13px;
        color: #edf4ff;
      }
      #${PANEL_ID} .aw-gen-number {
        width: 64px !important;
        box-sizing: border-box;
        border-radius: 10px;
        border: 1px solid #3b5775;
        padding: 6px 8px;
        background: #152334;
        color: #edf4ff;
        font: inherit;
        font-size: 13px;
        text-align: center;
        opacity: 1 !important;
        position: static !important;
      }
      #${PANEL_ID} .aw-gen-toggle {
        margin: 0;
      }
      #${PANEL_ID} .aw-gen-toggle[hidden] {
        display: none !important;
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
    panel.innerHTML = `
      <div class="aw-fill-card">
        <div class="aw-fill-header">
          <strong>Access Workspace</strong>
          <button type="button" class="aw-fill-close" aria-label="Dismiss">x</button>
        </div>
        <p class="aw-fill-copy">${isUpdate ? "Update the stored password for this portal credential." : "Save the credential you just used for this portal."}</p>
        <input class="aw-save-input aw-save-name" placeholder="Name">
        <input class="aw-save-input aw-save-url" placeholder="Portal URL">
        <input class="aw-save-input aw-save-username" placeholder="Username or email">
        <input class="aw-save-input aw-save-password" type="password" placeholder="Password">
        <label class="aw-save-checkbox">
          <input class="aw-save-personal" type="checkbox" ${draft.personal ? "checked" : ""} ${(isUpdate || !candidate.canSaveShared) ? "disabled" : ""}>
          <span>Save as personal password</span>
        </label>
        <p class="aw-fill-note"></p>
        <div class="aw-save-actions">
          <button type="button" class="aw-fill-action aw-save-submit">${isUpdate ? "Update password" : "Save credential"}</button>
          <button type="button" class="aw-save-cancel">Dismiss</button>
        </div>
        <p class="aw-fill-status" aria-live="polite"></p>
      </div>
    `;

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
    const filledPasswords = Array.from(form.querySelectorAll('input[type="password"]')).filter(
      (element) => element instanceof HTMLInputElement && String(element.value || "") !== ""
    );
    const passwordField = filledPasswords.find(isNewPasswordField) || filledPasswords[0] || null;
    const usernameField = loginFieldForPassword(passwordField, true) || firstFilledInput(form, [
      'input[type="email"]',
      'input[autocomplete="username"]',
      'input[name*="user" i]',
      'input[name*="email" i]',
      'input[id*="user" i]',
      'input[id*="email" i]',
      'input[type="text"]'
    ]);
    if (!(passwordField instanceof HTMLInputElement)) {
      return null;
    }
    const username = usernameField instanceof HTMLInputElement ? String(usernameField.value || "").trim() : "";
    const password = String(passwordField.value || "");
    const generated = password !== "" && password === lastGenerated;
    // A password we generated is worth saving even when no username field
    // was recognised; the save card lets the user fill it in.
    if (!password || (!username && !generated)) {
      return null;
    }
    return {
      name: document.title || window.location.hostname,
      title: document.title || "",
      url: window.location.href,
      username,
      password,
      generated
    };
  }

  async function refreshSuggestions() {
    scheduled = false;
    syncNavigationState();
    if (!isSupportedPage()) {
      removePanel();
      return;
    }
    if (workspaceOriginKnown === null) {
      await refreshWorkspaceOrigin();
    }
    if (isWorkspaceAppPage()) {
      removePanel();
      return;
    }
    // The save card comes first and ignores every dismissal: a credential
    // (above all a generated one) waiting to be saved must always surface.
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
        generatePanelDraft = null;
        await injectSavePanel(pendingSave);
        return;
      }
    } catch {
      // ignore save-prompt lookup failures and keep normal fill flow
    }
    if (dismissedURL === currentPageKey()) {
      removePanel();
      return;
    }
    const context = passwordContext();
    if (context.kind === "signup" || context.kind === "change") {
      // A form asking for a NEW password gets the generate chip, never the
      // login fill suggestions.
      if (generateDismissedKey === currentPageKey()) {
        if (document.getElementById(PANEL_ID)?.dataset.mode === PANEL_MODE_GENERATE) {
          removePanel();
        }
        return;
      }
      await injectGeneratePanel(context);
      return;
    }
    if (document.getElementById(PANEL_ID)?.dataset.mode === PANEL_MODE_GENERATE) {
      generatePanelDraft = null;
      removePanel();
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
      connectToken: data.payload?.connectToken,
      webAppOrigin: window.location.origin
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
  document.addEventListener("click", (event) => {
    if (isWorkspaceAppPage()) {
      return;
    }
    reportGeneratedOnSubmitIntent(event.target);
  }, true);
  document.addEventListener("submit", (event) => {
    if (isWorkspaceAppPage()) {
      return;
    }
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
