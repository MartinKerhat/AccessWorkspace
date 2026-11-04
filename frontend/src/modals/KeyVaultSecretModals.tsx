import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { api } from "../api/client";
import { defaultPassphraseOptions, generatePassphrase, type PassphraseOptions } from "../generator/passphrase";
import { defaultPasswordOptions, generatePassword, type PasswordOptions } from "../generator/password";
import { BYTE_OPTIONS, ENCODING_OPTIONS, Picker } from "../components/GeneratorPicker";
import { mergeStoredOptions } from "../generator/preferences";
import type { ByteEncoding } from "../generator/random";
import { generateKeyBytes } from "../generator/tokens";
import type {
  GeneratorCapabilities,
  KeyVaultCreateSecretInput,
  KeyVaultSecretStateInput,
  KeyVaultSecretValueInput,
  KeyVaultSecretVersion,
  Resource,
  WritableKeyVaultSource
} from "../types";
import { scrimDismissProps } from "./scrim";

// Key Vault's limit for one secret value, checked here so the user learns it
// before the upload, and again by the backend.
const MAX_SECRET_BYTES = 25 * 1024;
const SECRET_NAME_PATTERN = /^[0-9A-Za-z-]{1,127}$/;

type ValueMode = "typed" | "file";

// The value being written. Nothing here is persisted: it lives in the open
// modal, goes to the API once, and is dropped with the modal.
type ValueState = {
  mode: ValueMode;
  value: string;
  generated: boolean;
  contentType: string;
  fileName: string;
  fileSize: number;
  fileEncoding: "" | "utf-8" | "base64";
  // A file that is valid UTF-8 text may be stored as text or base64; a binary
  // one (a .pfx) only as base64.
  fileText: string | null;
  fileBase64: string;
  fileError: string;
};

type DatesState = {
  enabled: boolean;
  notBefore: string;
  expiresAt: string;
};

function emptyValueState(contentType = ""): ValueState {
  return {
    mode: "typed",
    value: "",
    generated: false,
    contentType,
    fileName: "",
    fileSize: 0,
    fileEncoding: "",
    fileText: null,
    fileBase64: "",
    fileError: ""
  };
}

function byteLength(value: string) {
  return new TextEncoder().encode(value).length;
}

// <input type="datetime-local"> works in local time without a zone; the API
// takes ISO instants.
function toLocalInput(iso?: string) {
  if (!iso) {
    return "";
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function fromLocalInput(value: string) {
  if (!value) {
    return undefined;
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}

function datesFromResource(resource?: Resource): DatesState {
  return {
    enabled: resource ? resource.status !== "disabled" : true,
    notBefore: toLocalInput(resource?.notBefore),
    expiresAt: toLocalInput(resource?.expiresAt)
  };
}

function datesError(dates: DatesState) {
  const from = fromLocalInput(dates.notBefore);
  const until = fromLocalInput(dates.expiresAt);
  if (from && until && new Date(until) <= new Date(from)) {
    return "Active until must be after active from.";
  }
  return "";
}

function contentTypeForFile(name: string, browserType: string) {
  const extension = name.toLowerCase().split(".").pop() ?? "";
  switch (extension) {
    case "pfx":
    case "p12":
      return "application/x-pkcs12";
    case "pem":
      return "application/x-pem-file";
    case "json":
      return "application/json";
    case "txt":
      return "text/plain";
    default:
      return browserType;
  }
}

function toBase64(bytes: Uint8Array) {
  let binary = "";
  for (let index = 0; index < bytes.length; index += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(index, index + 0x8000));
  }
  return btoa(binary);
}

function decodeText(bytes: Uint8Array): string | null {
  try {
    const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    return text.includes("\u0000") ? null : text;
  } catch {
    return null;
  }
}

function valueError(state: ValueState) {
  if (state.mode === "file") {
    if (state.fileError) {
      return state.fileError;
    }
    if (!state.fileName) {
      return "Choose a file.";
    }
  }
  if (!state.value) {
    return state.mode === "file" ? "" : "Enter or generate a value.";
  }
  const size = byteLength(state.value);
  if (size > MAX_SECRET_BYTES) {
    return state.fileEncoding === "base64"
      ? `Encoded as base64 the file takes ${size} bytes; Key Vault stores at most ${MAX_SECRET_BYTES}.`
      : `The value is ${size} bytes; Key Vault stores at most ${MAX_SECRET_BYTES}.`;
  }
  return "";
}

function valueInput(state: ValueState, dates: DatesState): KeyVaultSecretValueInput {
  const fromFile = state.mode === "file";
  return {
    value: state.value,
    contentType: state.contentType.trim(),
    fileEncoding: fromFile ? state.fileEncoding : "",
    fileName: fromFile ? state.fileName : "",
    inputMode: fromFile ? "file" : state.generated ? "generated" : "typed",
    enabled: dates.enabled,
    notBefore: fromLocalInput(dates.notBefore),
    expiresAt: fromLocalInput(dates.expiresAt)
  };
}

// The user's remembered Generator settings (Passwords and Keys parts), loaded
// once per modal; defaults until then — same behaviour as the object form.
function useRememberedGenerator(capabilities: GeneratorCapabilities) {
  const [passwordKind, setPasswordKind] = useState<"password" | "passphrase">("password");
  const [passwordOptions, setPasswordOptions] = useState<PasswordOptions>(defaultPasswordOptions);
  const [passphraseOptions, setPassphraseOptions] = useState<PassphraseOptions>(defaultPassphraseOptions);
  const [keyOptions, setKeyOptions] = useState<{ bytes: number; encoding: ByteEncoding }>({ bytes: 32, encoding: "base64" });
  const wanted = capabilities.passwords || capabilities.keysAndTokens;

  useEffect(() => {
    if (!wanted) {
      return;
    }
    void api
      .generatorPreferences()
      .then(({ preferences }) => {
        if (preferences.passwords?.kind === "passphrase" || preferences.passwords?.kind === "password") {
          setPasswordKind(preferences.passwords.kind);
        }
        setPasswordOptions((current) => mergeStoredOptions(current, preferences.password));
        setPassphraseOptions((current) => mergeStoredOptions(current, preferences.passphrase));
        const key = mergeStoredOptions({ bytes: 32, encoding: "base64" as string }, preferences.key);
        setKeyOptions({
          bytes: [16, 24, 32, 48, 64, 128].includes(key.bytes) ? key.bytes : 32,
          encoding: (["hex", "base64", "base64url"].includes(key.encoding) ? key.encoding : "base64") as ByteEncoding
        });
      })
      .catch(() => undefined);
  }, [wanted]);

  return {
    password() {
      try {
        return passwordKind === "passphrase" ? generatePassphrase(passphraseOptions) : generatePassword(passwordOptions);
      } catch {
        return passwordKind === "passphrase" ? generatePassphrase(defaultPassphraseOptions) : generatePassword(defaultPasswordOptions);
      }
    },
    passwordLabel: passwordKind === "passphrase" ? "Generate passphrase" : "Generate password",
    keyOptions,
    setKeyOptions,
    key() {
      return generateKeyBytes(keyOptions.bytes, keyOptions.encoding);
    }
  };
}

function SecretValueField({
  state,
  setState,
  generatorCapabilities
}: {
  state: ValueState;
  setState: (update: (current: ValueState) => ValueState) => void;
  generatorCapabilities: GeneratorCapabilities;
}) {
  const generator = useRememberedGenerator(generatorCapabilities);
  const [openPicker, setOpenPicker] = useState<string | null>(null);
  const size = state.value ? byteLength(state.value) : 0;

  async function loadFile(file: File | undefined) {
    if (!file) {
      return;
    }
    const bytes = new Uint8Array(await file.arrayBuffer());
    const text = decodeText(bytes);
    const base64 = toBase64(bytes);
    setState((current) => ({
      ...current,
      mode: "file",
      generated: false,
      fileName: file.name,
      fileSize: bytes.length,
      fileText: text,
      fileBase64: base64,
      fileEncoding: text !== null ? "utf-8" : "base64",
      value: text ?? base64,
      contentType: contentTypeForFile(file.name, file.type) || current.contentType,
      fileError: bytes.length === 0 ? "The file is empty." : ""
    }));
  }

  function setEncoding(encoding: "utf-8" | "base64") {
    setState((current) =>
      current.fileText === null && encoding === "utf-8"
        ? current
        : { ...current, fileEncoding: encoding, value: encoding === "utf-8" ? current.fileText ?? "" : current.fileBase64 }
    );
  }

  return (
    <div className="keyvault-value-field wide">
      <div className="segmented-control" role="tablist" aria-label="Value source">
        {(["typed", "file"] as const).map((mode) => (
          <button
            key={mode}
            type="button"
            role="tab"
            aria-selected={state.mode === mode}
            className={`segmented-button ${state.mode === mode ? "active" : ""}`}
            onClick={() => setState((current) => (current.mode === mode ? current : { ...emptyValueState(current.contentType), mode }))}
          >
            {mode === "typed" ? "Type, paste or generate" : "Upload a file"}
          </button>
        ))}
      </div>

      {state.mode === "typed" ? (
        <>
          <textarea
            rows={4}
            value={state.value}
            spellCheck={false}
            autoComplete="off"
            aria-label="Secret value"
            placeholder="The secret value"
            onChange={(event) => {
              const value = event.target.value;
              setState((current) => ({ ...current, value, generated: false }));
            }}
          />
          {generatorCapabilities.keysAndTokens ? (
            <div className="form-grid">
              <label>
                <span>Key size</span>
                <Picker
                  id="bytes"
                  openId={openPicker}
                  setOpenId={setOpenPicker}
                  value={String(generator.keyOptions.bytes)}
                  options={BYTE_OPTIONS}
                  onSelect={(next) => generator.setKeyOptions((current) => ({ ...current, bytes: Number(next) }))}
                />
              </label>
              <label>
                <span>Key encoding</span>
                <Picker
                  id="encoding"
                  openId={openPicker}
                  setOpenId={setOpenPicker}
                  value={generator.keyOptions.encoding}
                  options={ENCODING_OPTIONS}
                  onSelect={(next) => generator.setKeyOptions((current) => ({ ...current, encoding: next }))}
                />
              </label>
            </div>
          ) : null}
          {generatorCapabilities.passwords || generatorCapabilities.keysAndTokens ? (
            <div className="action-row compact-actions">
              {generatorCapabilities.passwords ? (
                <button
                  type="button"
                  className="button ghost"
                  onClick={() => setState((current) => ({ ...current, value: generator.password(), generated: true }))}
                >
                  {generator.passwordLabel}
                </button>
              ) : null}
              {generatorCapabilities.keysAndTokens ? (
                <button
                  type="button"
                  className="button ghost"
                  onClick={() => setState((current) => ({ ...current, value: generator.key(), generated: true }))}
                >
                  Generate key
                </button>
              ) : null}
            </div>
          ) : null}
          <p className="selection-hint">
            {state.generated ? "Generated in your browser. " : ""}
            {size > 0 ? `${size} of ${MAX_SECRET_BYTES} bytes.` : `Up to ${MAX_SECRET_BYTES} bytes.`} The value goes straight to Azure and is
            never stored in the workspace.
          </p>
        </>
      ) : (
        <>
          <input type="file" aria-label="Secret file" onChange={(event) => void loadFile(event.target.files?.[0])} />
          {state.fileName ? (
            <>
              <p className="selection-hint">
                {state.fileName} · {state.fileSize} bytes{state.fileText === null ? " · binary" : " · text"}
              </p>
              <div className="segmented-control" role="tablist" aria-label="File encoding">
                {(["utf-8", "base64"] as const).map((encoding) => (
                  <button
                    key={encoding}
                    type="button"
                    role="tab"
                    disabled={encoding === "utf-8" && state.fileText === null}
                    aria-selected={state.fileEncoding === encoding}
                    className={`segmented-button ${state.fileEncoding === encoding ? "active" : ""}`}
                    onClick={() => setEncoding(encoding)}
                  >
                    {encoding === "utf-8" ? "Store as text (utf-8)" : "Store as base64"}
                  </button>
                ))}
              </div>
            </>
          ) : null}
          <p className="selection-hint">
            Stored the way <code>az keyvault secret set --file … --encoding</code> stores it, including the <code>file-encoding</code>{" "}
            tag, so <code>az keyvault secret download</code> gives back the original file. Binary files (.pfx, .p12) are always
            base64. Key Vault holds at most {MAX_SECRET_BYTES} bytes after encoding.
          </p>
        </>
      )}
    </div>
  );
}

function DatesFields({ dates, setDates }: { dates: DatesState; setDates: (update: (current: DatesState) => DatesState) => void }) {
  return (
    <>
      <label className="checkbox wide">
        <input type="checkbox" checked={dates.enabled} onChange={(event) => setDates((current) => ({ ...current, enabled: event.target.checked }))} />
        <span>Enabled — a disabled secret cannot be read by anyone, apps included</span>
      </label>
      <label>
        <span>Active from</span>
        <input
          type="datetime-local"
          value={dates.notBefore}
          onChange={(event) => setDates((current) => ({ ...current, notBefore: event.target.value }))}
        />
      </label>
      <label>
        <span>Active until</span>
        <input
          type="datetime-local"
          value={dates.expiresAt}
          onChange={(event) => setDates((current) => ({ ...current, expiresAt: event.target.value }))}
        />
      </label>
      <p className="selection-hint wide">
        Leave a date empty for none. Azure treats these dates as information, not as a lock: the secret can still be read outside
        them. The workspace shows the window and sends expiry reminders; only Enabled actually blocks reads.
      </p>
    </>
  );
}

function ModalFrame({ eyebrow, title, message, onClose, children }: { eyebrow: string; title: string; message?: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="modal-scrim" {...scrimDismissProps(onClose)}>
      <div className="modal-card keyvault-secret-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">{eyebrow}</p>
            <h2>{title}</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        {message ? <div className="banner compact">{message}</div> : null}
        {children}
      </div>
    </div>
  );
}

export function KeyVaultCreateSecretModal({
  sources,
  groups,
  generatorCapabilities,
  message,
  busy,
  onSubmit,
  onClose
}: {
  sources: WritableKeyVaultSource[];
  groups: string[];
  generatorCapabilities: GeneratorCapabilities;
  message?: string;
  busy: boolean;
  onSubmit: (input: KeyVaultCreateSecretInput) => void;
  onClose: () => void;
}) {
  const [vaultUrl, setVaultUrl] = useState(sources[0]?.vaultUrl ?? "");
  const [secretName, setSecretName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [environment, setEnvironment] = useState("");
  const [ownerTeam, setOwnerTeam] = useState("");
  const [description, setDescription] = useState("");
  const [notes, setNotes] = useState("");
  const [allowedGroups, setAllowedGroups] = useState<string[]>([]);
  const [value, setValue] = useState<ValueState>(emptyValueState());
  const [dates, setDates] = useState<DatesState>(datesFromResource());

  const nameError = secretName && !SECRET_NAME_PATTERN.test(secretName) ? "Letters, digits and dashes only, at most 127." : "";
  const problem = !vaultUrl ? "Choose a vault." : !secretName ? "Enter the secret name." : nameError || valueError(value) || datesError(dates);

  return (
    <ModalFrame eyebrow="Key Vault" title="New secret in Azure" message={message} onClose={onClose}>
      {sources.length === 0 ? (
        <p className="section-copy">
          No vault allows writes from the workspace yet. An administrator turns this on per vault in Administration → Azure → Key
          Vault sources.
        </p>
      ) : (
        <div className="form-grid">
          <label>
            <span>Vault</span>
            <select value={vaultUrl} onChange={(event) => setVaultUrl(event.target.value)}>
              {sources.map((source) => (
                <option key={source.vaultUrl} value={source.vaultUrl}>
                  {source.name || source.vaultUrl}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>Secret name in Azure</span>
            <input value={secretName} spellCheck={false} onChange={(event) => setSecretName(event.target.value.trim())} placeholder="e.g. erp-api-key" />
            {nameError ? <span className="error-copy">{nameError}</span> : null}
          </label>
          <SecretValueField state={value} setState={setValue} generatorCapabilities={generatorCapabilities} />
          <label>
            <span>Content type (optional)</span>
            <input value={value.contentType} onChange={(event) => setValue((current) => ({ ...current, contentType: event.target.value }))} placeholder="e.g. text/plain" />
          </label>
          <DatesFields dates={dates} setDates={setDates} />
          <label>
            <span>Name in the workspace (optional)</span>
            <input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder={secretName || "defaults to the secret name"} />
          </label>
          <label>
            <span>Environment (optional)</span>
            <input value={environment} onChange={(event) => setEnvironment(event.target.value)} />
          </label>
          <label>
            <span>Owner team (optional)</span>
            <select value={ownerTeam} onChange={(event) => setOwnerTeam(event.target.value)}>
              <option value="">none</option>
              {groups.map((group) => (
                <option key={group} value={group}>
                  {group}
                </option>
              ))}
            </select>
          </label>
          <label className="wide">
            <span>Description (optional)</span>
            <textarea rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />
          </label>
          <label className="wide">
            <span>Notes (optional)</span>
            <textarea rows={2} value={notes} onChange={(event) => setNotes(event.target.value)} />
          </label>
          <div className="wide">
            <span>Shared with</span>
            <div className="keyvault-group-list">
              {groups.length === 0 ? <span className="selection-hint">No groups available — everyone with Key Vault access will see it.</span> : null}
              {groups.map((group) => (
                <label key={group} className="checkbox">
                  <input
                    type="checkbox"
                    checked={allowedGroups.includes(group)}
                    onChange={() =>
                      setAllowedGroups((current) => (current.includes(group) ? current.filter((item) => item !== group) : [...current, group]))
                    }
                  />
                  <span>{group}</span>
                </label>
              ))}
            </div>
            <span className="selection-hint">{allowedGroups.length === 0 ? "No group selected: everyone with Key Vault access." : ""} You become the owner.</span>
          </div>
        </div>
      )}
      <div className="action-row">
        <button
          className="button primary"
          disabled={busy || sources.length === 0 || Boolean(problem)}
          title={problem || undefined}
          onClick={() =>
            onSubmit({
              ...valueInput(value, dates),
              vaultUrl,
              secretName,
              name: displayName.trim(),
              description,
              ownerTeam,
              environment,
              notes,
              allowedGroups
            })
          }
        >
          Create in Azure
        </button>
        <button className="button ghost" onClick={onClose}>
          Cancel
        </button>
        {problem && sources.length > 0 ? <span className="selection-hint">{problem}</span> : null}
      </div>
    </ModalFrame>
  );
}

export function KeyVaultSetValueModal({
  resource,
  generatorCapabilities,
  message,
  busy,
  onSubmit,
  onClose
}: {
  resource: Resource;
  generatorCapabilities: GeneratorCapabilities;
  message?: string;
  busy: boolean;
  onSubmit: (input: KeyVaultSecretValueInput) => void;
  onClose: () => void;
}) {
  const [value, setValue] = useState<ValueState>(emptyValueState(resource.contentType));
  // A new version starts from the current state (maintainer decision
  // 2026-11-04): same enabled flag and window, editable before saving.
  const [dates, setDates] = useState<DatesState>(datesFromResource(resource));
  const problem = valueError(value) || datesError(dates);

  return (
    <ModalFrame eyebrow="Key Vault" title={`New value for ${resource.objectName || resource.name}`} message={message} onClose={onClose}>
      <p className="section-copy">
        Azure keeps the current value as an older version; everything that reads this secret without a pinned version gets the new
        value right away. Tags carry over to the new version.
      </p>
      <div className="form-grid">
        <SecretValueField state={value} setState={setValue} generatorCapabilities={generatorCapabilities} />
        <label>
          <span>Content type (optional)</span>
          <input value={value.contentType} onChange={(event) => setValue((current) => ({ ...current, contentType: event.target.value }))} />
        </label>
        <DatesFields dates={dates} setDates={setDates} />
      </div>
      <div className="action-row">
        <button className="button primary" disabled={busy || Boolean(problem)} title={problem || undefined} onClick={() => onSubmit(valueInput(value, dates))}>
          Save new value in Azure
        </button>
        <button className="button ghost" onClick={onClose}>
          Cancel
        </button>
        {problem ? <span className="selection-hint">{problem}</span> : null}
      </div>
    </ModalFrame>
  );
}

export function KeyVaultStateModal({
  resource,
  version,
  message,
  busy,
  onSubmit,
  onClose
}: {
  resource: Resource;
  version: KeyVaultSecretVersion;
  message?: string;
  busy: boolean;
  onSubmit: (input: KeyVaultSecretStateInput) => void;
  onClose: () => void;
}) {
  const [dates, setDates] = useState<DatesState>({
    enabled: version.enabled,
    notBefore: toLocalInput(version.notBefore),
    expiresAt: toLocalInput(version.expiresAt)
  });
  const problem = datesError(dates);

  return (
    <ModalFrame
      eyebrow="Key Vault"
      title={`${resource.objectName || resource.name} · version ${version.version.slice(0, 8)}`}
      message={message}
      onClose={onClose}
    >
      <p className="section-copy">
        Changes {version.current ? "the current version" : "this older version"} in Azure. The change is recorded in the audit
        trail with the previous state.
      </p>
      <div className="form-grid">
        <DatesFields dates={dates} setDates={setDates} />
      </div>
      <div className="action-row">
        <button
          className="button primary"
          disabled={busy || Boolean(problem)}
          title={problem || undefined}
          onClick={() =>
            onSubmit({
              enabled: dates.enabled,
              notBefore: fromLocalInput(dates.notBefore),
              expiresAt: fromLocalInput(dates.expiresAt)
            })
          }
        >
          Save in Azure
        </button>
        <button className="button ghost" onClick={onClose}>
          Cancel
        </button>
        {problem ? <span className="selection-hint">{problem}</span> : null}
      </div>
    </ModalFrame>
  );
}
