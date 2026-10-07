import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { CertificateInput, CertificateResult, GeneratorCapabilities, GeneratorPreferences } from "../types";
import { defaultPasswordOptions, generatePassword, passwordEntropyBits, type PasswordOptions } from "../generator/password";
import { defaultPassphraseOptions, generatePassphrase, passphraseEntropyBits, type PassphraseOptions } from "../generator/passphrase";
import { generateKeyBytes, generateToken, generateUUID } from "../generator/tokens";
import type { ByteEncoding } from "../generator/random";
import { fromBase64 } from "../generator/random";
import { ed25519Supported, generateECKeypair, generateEd25519SSHKeypair, generateRSAKeypair } from "../generator/keypairs";

type Category = "passwords" | "keys" | "keypairs" | "certificates";
type PasswordKind = "password" | "passphrase";
type KeyKind = "key" | "token" | "uuid";
type PairKind = "rsa" | "ec" | "ssh";

const CATEGORIES: { id: Category; label: string; copy: string }[] = [
  { id: "passwords", label: "Passwords", copy: "Random passwords and word-based passphrases for accounts, mailboxes and service logins." },
  { id: "keys", label: "Keys & tokens", copy: "Raw key material, API tokens and identifiers for application configuration." },
  { id: "keypairs", label: "Keypairs", copy: "RSA, EC and SSH keypairs in the formats servers, Git hosting and signing libraries expect." },
  { id: "certificates", label: "Certificates", copy: "Self-signed certificates with the private key as PEM and a password-protected PFX." }
];

type PickerOption<T extends string> = { value: T; label: string; description?: string };

const BYTE_OPTIONS: PickerOption<string>[] = [
  { value: "16", label: "16 bytes (128 bits)", description: "AES-128" },
  { value: "24", label: "24 bytes (192 bits)", description: "AES-192" },
  { value: "32", label: "32 bytes (256 bits)", description: "AES-256, HMAC-SHA256, Fernet — 64 hex characters" },
  { value: "48", label: "48 bytes (384 bits)", description: "HMAC-SHA384" },
  { value: "64", label: "64 bytes (512 bits)", description: "HMAC-SHA512, JWT HS512 secrets" },
  { value: "128", label: "128 bytes (1024 bits)", description: "Libraries that insist on very long secrets" }
];

const ENCODING_OPTIONS: PickerOption<ByteEncoding>[] = [
  { value: "hex", label: "Hex", description: "0-9 a-f, two characters per byte" },
  { value: "base64", label: "Base64", description: "Standard alphabet with padding" },
  { value: "base64url", label: "Base64url", description: "URL-safe alphabet, no padding — Fernet, JWT secrets" }
];

const SEPARATOR_OPTIONS: PickerOption<string>[] = [
  { value: "-", label: "Dash  ( - )", description: "Correct-Horse-Battery" },
  { value: " ", label: "Space", description: "Correct Horse Battery" },
  { value: ".", label: "Dot  ( . )", description: "Correct.Horse.Battery" },
  { value: "_", label: "Underscore  ( _ )", description: "Correct_Horse_Battery" },
  { value: "", label: "None", description: "CorrectHorseBattery" }
];

const RSA_OPTIONS: PickerOption<string>[] = [
  { value: "2048", label: "2048 bits", description: "Common default" },
  { value: "3072", label: "3072 bits", description: "Middle ground; rarely required" },
  { value: "4096", label: "4096 bits", description: "Slower to generate" }
];

const CURVE_OPTIONS: PickerOption<"P-256" | "P-384">[] = [
  { value: "P-256", label: "P-256", description: "JWT ES256, most TLS" },
  { value: "P-384", label: "P-384", description: "JWT ES384" }
];

const PROFILE_OPTIONS: PickerOption<CertificateInput["profile"]>[] = [
  { value: "document_signing", label: "Document signing", description: "PDF and e-mail signing — digital signature + non-repudiation" },
  { value: "code_signing", label: "Code signing", description: "Signing executables, scripts, packages" },
  { value: "tls_server", label: "TLS server", description: "HTTPS for an internal host — add DNS names / IPs" },
  { value: "client_auth", label: "Client authentication", description: "Mutual TLS, VPN or API client identity" }
];

const KEY_ALGO_OPTIONS: PickerOption<CertificateInput["keyAlgorithm"]>[] = [
  { value: "rsa2048", label: "RSA 2048", description: "Widest compatibility" },
  { value: "rsa4096", label: "RSA 4096", description: "Stronger and slower; larger signatures" },
  { value: "ec_p256", label: "EC P-256", description: "Small and fast; needs modern software" }
];

const PFX_OPTIONS: PickerOption<CertificateInput["pfxEncoding"]>[] = [
  { value: "modern", label: "Modern", description: "AES-256 / SHA-256 — Windows 10+, OpenSSL 3" },
  { value: "legacy", label: "Legacy", description: "3DES / RC2 / SHA-1 — older software that cannot open modern PFX files" }
];

function download(fileName: string, content: string | Uint8Array, mime: string) {
  const blob = new Blob([content as BlobPart], { type: mime });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// Single-value picker in the app's house style (same markup and classes as
// the object form), with one open picker at a time per page.
function Picker<T extends string>({
  id,
  openId,
  setOpenId,
  value,
  options,
  onSelect
}: {
  id: string;
  openId: string | null;
  setOpenId: (id: string | null) => void;
  value: T;
  options: PickerOption<T>[];
  onSelect: (value: T) => void;
}) {
  const open = openId === id;
  const selected = options.find((option) => option.value === value);
  return (
    <div className="picker-shell">
      <button type="button" className="single-picker-trigger" onClick={() => setOpenId(open ? null : id)}>
        <span>{selected?.label ?? value}</span>
        <span>{open ? "Close" : "Select"}</span>
      </button>
      {open ? (
        <div className="group-picker-dropdown">
          <div className="picker-option-list">
            {options.map((option) => (
              <button
                key={option.value}
                type="button"
                className={`picker-option ${value === option.value ? "active" : ""}`}
                onClick={() => {
                  onSelect(option.value);
                  setOpenId(null);
                }}
              >
                <strong>{option.label}</strong>
                {option.description ? <span>{option.description}</span> : null}
              </button>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function Segmented<T extends string>({ value, options, onChange, label }: { value: T; options: { value: T; label: string }[]; onChange: (value: T) => void; label: string }) {
  return (
    <div className="segmented-control" role="tablist" aria-label={label}>
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={option.value === value}
          className={`segmented-button ${option.value === value ? "active" : ""}`}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

// The generated value itself is a button: one click copies it.
function GeneratedValue({ value, onCopied }: { value: string; onCopied: (text: string) => void }) {
  return (
    <button
      type="button"
      className="generator-value"
      title="Click to copy"
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => onCopied("Copied to clipboard"));
      }}
    >
      {value}
    </button>
  );
}

function CopyButton({ value, label, onCopied }: { value: string; label: string; onCopied: (text: string) => void }) {
  return (
    <button
      type="button"
      className="button ghost"
      disabled={!value}
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => onCopied("Copied to clipboard"));
      }}
    >
      {label}
    </button>
  );
}

function OutputBlock({ title, value, fileName, mime = "text/plain", onCopied }: { title: string; value: string; fileName?: string; mime?: string; onCopied: (text: string) => void }) {
  return (
    <div className="generator-output-block">
      <div className="generator-output-header">
        <span>{title}</span>
        <div className="generator-output-actions">
          <button type="button" className="button ghost compact-button" disabled={!value} onClick={() => void navigator.clipboard.writeText(value).then(() => onCopied("Copied to clipboard"))}>
            Copy
          </button>
          {fileName ? (
            <button type="button" className="button ghost compact-button" disabled={!value} onClick={() => download(fileName, value, mime)}>
              Download
            </button>
          ) : null}
        </div>
      </div>
      <pre
        className="generator-output"
        title="Click to copy"
        onClick={() => {
          if (value) {
            void navigator.clipboard.writeText(value).then(() => onCopied("Copied to clipboard"));
          }
        }}
      >
        {value}
      </pre>
    </div>
  );
}

// What each purpose needs from the user. The purpose is chosen first and
// the form below adapts: the identity a certificate carries is different for
// a person signing documents, a publisher signing code, a host serving TLS
// and a client proving who it is.
type PurposeGuide = {
  intro: string;
  cnLabel: string;
  cnPlaceholder: string;
  email: "required" | "optional" | "hidden";
  emailLabel: string;
  showOrganization: boolean;
  showUnit: boolean;
  showCountry: boolean;
  showSans: boolean;
};

const PURPOSE_GUIDES: Record<CertificateInput["profile"], PurposeGuide> = {
  document_signing: {
    intro:
      "Identifies a person. The signer's name and e-mail are shown in the signature panel of a PDF or an ERP document; organisation, department and country appear next to them.",
    cnLabel: "Signer's full name",
    cnPlaceholder: "Jana Nováková",
    email: "required",
    emailLabel: "Signer's e-mail",
    showOrganization: true,
    showUnit: true,
    showCountry: true,
    showSans: false
  },
  code_signing: {
    intro: "Identifies a publisher. Windows shows the name below as the publisher when a signed program or script is run.",
    cnLabel: "Publisher name",
    cnPlaceholder: "Example Org Tools",
    email: "hidden",
    emailLabel: "",
    showOrganization: true,
    showUnit: false,
    showCountry: true,
    showSans: false
  },
  tls_server: {
    intro:
      "Identifies a host. Browsers check only the host names and IP addresses, so list every name the server is reached by; the first one is also used as the common name.",
    cnLabel: "Primary host name",
    cnPlaceholder: "app.example.internal",
    email: "hidden",
    emailLabel: "",
    showOrganization: true,
    showUnit: false,
    showCountry: false,
    showSans: true
  },
  client_auth: {
    intro:
      "Identifies a user or device to a server (mutual TLS, VPN, API clients). The server decides what it expects here — usually a user name, device name or e-mail.",
    cnLabel: "User or device identity",
    cnPlaceholder: "jana.novakova or laptop-0042",
    email: "optional",
    emailLabel: "E-mail (if the server identifies users by e-mail)",
    showOrganization: true,
    showUnit: true,
    showCountry: false,
    showSans: false
  }
};

const defaultCertificateInput: CertificateInput = {
  commonName: "",
  organization: "",
  organizationalUnit: "",
  country: "",
  email: "",
  validityDays: 365,
  keyAlgorithm: "rsa2048",
  profile: "document_signing",
  dnsNames: [],
  pfxPassword: "",
  pfxEncoding: "modern"
};

type Props = {
  busy: boolean;
  onMessage: (message: string | undefined) => void;
  // Which categories this user may use (rights generator.*). Categories the
  // user lacks are not rendered at all.
  allowed: GeneratorCapabilities;
};

const CATEGORY_ALLOWED: Record<Category, keyof GeneratorCapabilities> = {
  passwords: "passwords",
  keys: "keysAndTokens",
  keypairs: "keypairs",
  certificates: "certificates"
};

function clampNumber(value: number, min: number, max: number): number {
  if (!Number.isFinite(value)) {
    return min;
  }
  return Math.min(max, Math.max(min, Math.round(value)));
}

// Slider plus an editable number box for the same value: drag for feel,
// type for precision.
function RangeWithNumber({ label, value, min, max, onChange, wide = true }: { label: string; value: number; min: number; max: number; onChange: (value: number) => void; wide?: boolean }) {
  const [draft, setDraft] = useState(String(value));
  useEffect(() => {
    setDraft(String(value));
  }, [value]);
  const commit = () => {
    const next = clampNumber(Number(draft), min, max);
    setDraft(String(next));
    onChange(next);
  };
  return (
    <label className={wide ? "wide" : undefined}>
      <span>{label}</span>
      <div className="generator-range-row">
        <input type="range" min={min} max={max} value={value} onChange={(event) => onChange(Number(event.target.value))} />
        <input
          type="number"
          min={min}
          max={max}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              commit();
            }
          }}
        />
      </div>
    </label>
  );
}

// Settings documents remembered per user and part. Only generation options
// are kept — never generated values, names, e-mails or passwords.
function pick<T extends object>(source: Record<string, unknown> | undefined, keys: (keyof T)[], fallback: T): T {
  if (!source) {
    return fallback;
  }
  const out = { ...fallback };
  for (const key of keys) {
    const value = source[key as string];
    if (value !== undefined && typeof value === typeof fallback[key]) {
      (out as Record<string, unknown>)[key as string] = value;
    }
  }
  return out;
}

// Generator: secrets and keys produced in the browser, plus a server-made
// self-signed certificate (the one thing the browser cannot package as PFX).
// Nothing is stored anywhere; the user copies or downloads what they need.
export function GeneratorPage({ busy, onMessage, allowed }: Props) {
  const categories = CATEGORIES.filter((item) => allowed[CATEGORY_ALLOWED[item.id]]);
  const [category, setCategory] = useState<Category>(categories[0]?.id ?? "passwords");
  // Remembered settings: loaded once, applied before the first generation;
  // saved whenever the user actually uses a part (regenerate, copy, generate).
  const prefsLoadedRef = useRef(false);
  const [prefsReady, setPrefsReady] = useState(false);
  const [passwordKind, setPasswordKind] = useState<PasswordKind>("password");
  const [keyKind, setKeyKind] = useState<KeyKind>("key");
  const [pairKind, setPairKind] = useState<PairKind>("rsa");
  const [openPicker, setOpenPicker] = useState<string | null>(null);

  // Simple values
  const [passwordOptions, setPasswordOptions] = useState<PasswordOptions>(defaultPasswordOptions);
  const [passphraseOptions, setPassphraseOptions] = useState<PassphraseOptions>(defaultPassphraseOptions);
  const [keyBytes, setKeyBytes] = useState(32);
  const [keyEncoding, setKeyEncoding] = useState<ByteEncoding>("hex");
  const [tokenLength, setTokenLength] = useState(40);
  const [tokenPrefix, setTokenPrefix] = useState("");
  const [value, setValue] = useState("");

  // Keypairs
  const [rsaBits, setRsaBits] = useState<2048 | 3072 | 4096>(2048);
  const [ecCurve, setEcCurve] = useState<"P-256" | "P-384">("P-256");
  const [sshComment, setSshComment] = useState("");
  const [sshAvailable, setSshAvailable] = useState<boolean | null>(null);
  const [pairOutput, setPairOutput] = useState<{ private: string; public: string; privateName: string; publicName: string } | null>(null);
  const [working, setWorking] = useState(false);

  // Certificate
  const [certInput, setCertInput] = useState<CertificateInput>(defaultCertificateInput);
  const [certDnsNames, setCertDnsNames] = useState("");
  const [certResult, setCertResult] = useState<CertificateResult | null>(null);
  const [certError, setCertError] = useState<string | undefined>();

  useEffect(() => {
    void ed25519Supported().then(setSshAvailable);
  }, []);

  useEffect(() => {
    if (prefsLoadedRef.current) {
      return;
    }
    prefsLoadedRef.current = true;
    void api
      .generatorPreferences()
      .then(({ preferences }) => applyPreferences(preferences))
      .catch(() => {
        // Non-fatal: defaults stay.
      })
      .finally(() => setPrefsReady(true));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function applyPreferences(preferences: GeneratorPreferences) {
    setPasswordOptions((c) => pick(preferences.password, ["length", "lowercase", "uppercase", "digits", "symbols", "avoidAmbiguous", "safeSymbols"], c));
    setPassphraseOptions((c) => pick(preferences.passphrase, ["words", "separator", "capitalize", "addDigit"], c));
    const key = pick(preferences.key, ["bytes", "encoding"], { bytes: keyBytes, encoding: keyEncoding as string });
    if ([16, 24, 32, 48, 64, 128].includes(key.bytes)) {
      setKeyBytes(key.bytes);
    }
    if (["hex", "base64", "base64url"].includes(key.encoding)) {
      setKeyEncoding(key.encoding as ByteEncoding);
    }
    const token = pick(preferences.token, ["length", "prefix"], { length: tokenLength, prefix: tokenPrefix });
    setTokenLength(clampNumber(token.length, 16, 96));
    setTokenPrefix(token.prefix);
    const rsa = pick(preferences.rsa, ["bits"], { bits: rsaBits as number });
    if ([2048, 3072, 4096].includes(rsa.bits)) {
      setRsaBits(rsa.bits as 2048 | 3072 | 4096);
    }
    const ec = pick(preferences.ec, ["curve"], { curve: ecCurve as string });
    if (ec.curve === "P-256" || ec.curve === "P-384") {
      setEcCurve(ec.curve);
    }
    const cert = pick(preferences.certificate, ["keyAlgorithm", "validityDays", "pfxEncoding", "organization", "country", "profile"], {
      keyAlgorithm: certInput.keyAlgorithm as string,
      validityDays: certInput.validityDays,
      pfxEncoding: certInput.pfxEncoding as string,
      organization: certInput.organization,
      country: certInput.country,
      profile: certInput.profile as string
    });
    setCertInput((c) => ({
      ...c,
      keyAlgorithm: (["rsa2048", "rsa4096", "ec_p256"].includes(cert.keyAlgorithm) ? cert.keyAlgorithm : c.keyAlgorithm) as CertificateInput["keyAlgorithm"],
      validityDays: clampNumber(cert.validityDays, 1, 3650),
      pfxEncoding: (cert.pfxEncoding === "legacy" ? "legacy" : "modern") as CertificateInput["pfxEncoding"],
      organization: cert.organization,
      country: cert.country,
      profile: (cert.profile in PURPOSE_GUIDES ? cert.profile : c.profile) as CertificateInput["profile"]
    }));
  }

  // Fire-and-forget: the server keeps the last used settings per part.
  function remember(part: string, settings: Record<string, unknown>) {
    void api.saveGeneratorPreference(part, settings).catch(() => undefined);
  }

  function rememberCurrentSimple() {
    if (category === "passwords") {
      if (passwordKind === "password") {
        remember("password", { ...passwordOptions });
      } else {
        remember("passphrase", { ...passphraseOptions });
      }
    } else if (category === "keys") {
      if (keyKind === "key") {
        remember("key", { bytes: keyBytes, encoding: keyEncoding });
      } else if (keyKind === "token") {
        remember("token", { length: tokenLength, prefix: tokenPrefix });
      }
    }
  }

  // Close an open picker on any click outside it.
  useEffect(() => {
    if (!openPicker) {
      return;
    }
    function handlePointerDown(event: PointerEvent) {
      const target = event.target as Element | null;
      if (target && !target.closest(".picker-shell")) {
        setOpenPicker(null);
      }
    }
    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, [openPicker]);

  function regenerateAndRemember() {
    regenerateSimple();
    rememberCurrentSimple();
  }

  function regenerateSimple() {
    try {
      if (category === "passwords") {
        setValue(passwordKind === "password" ? generatePassword(passwordOptions) : generatePassphrase(passphraseOptions));
      } else if (category === "keys") {
        setValue(keyKind === "key" ? generateKeyBytes(keyBytes, keyEncoding) : keyKind === "token" ? generateToken(tokenLength, tokenPrefix) : generateUUID());
      }
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Generating failed");
    }
  }

  // Any change of category, variant or option produces a fresh value for the
  // simple kinds, so the page never shows an empty or stale box.
  useEffect(() => {
    setOpenPicker(null);
    regenerateSimple();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [category, passwordKind, keyKind, passwordOptions, passphraseOptions, keyBytes, keyEncoding, tokenLength, tokenPrefix]);

  useEffect(() => {
    setPairOutput(null);
  }, [pairKind, category]);

  async function generatePair() {
    setWorking(true);
    try {
      if (pairKind === "rsa") {
        remember("rsa", { bits: rsaBits });
      } else if (pairKind === "ec") {
        remember("ec", { curve: ecCurve });
      }
      if (pairKind === "rsa") {
        const pair = await generateRSAKeypair(rsaBits);
        setPairOutput({ private: pair.privateKeyPem, public: pair.publicKeyPem, privateName: `rsa-${rsaBits}-private.pem`, publicName: `rsa-${rsaBits}-public.pem` });
      } else if (pairKind === "ec") {
        const pair = await generateECKeypair(ecCurve);
        setPairOutput({ private: pair.privateKeyPem, public: pair.publicKeyPem, privateName: `ec-${ecCurve.toLowerCase()}-private.pem`, publicName: `ec-${ecCurve.toLowerCase()}-public.pem` });
      } else {
        const pair = await generateEd25519SSHKeypair(sshComment);
        setPairOutput({ private: pair.privateKeyOpenSSH, public: pair.publicKeyLine, privateName: "id_ed25519", publicName: "id_ed25519.pub" });
      }
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Generating the keypair failed");
    } finally {
      setWorking(false);
    }
  }

  async function generateCertificate() {
    setCertError(undefined);
    setWorking(true);
    try {
      const input: CertificateInput = {
        ...certInput,
        dnsNames: certDnsNames
          .split(/[\s,;]+/)
          .map((name) => name.trim())
          .filter(Boolean)
      };
      setCertResult(await api.generateCertificate(input));
      remember("certificate", {
        profile: certInput.profile,
        keyAlgorithm: certInput.keyAlgorithm,
        validityDays: certInput.validityDays,
        pfxEncoding: certInput.pfxEncoding,
        organization: certInput.organization,
        country: certInput.country
      });
    } catch (error) {
      setCertError(error instanceof Error ? error.message : "Generating the certificate failed");
    } finally {
      setWorking(false);
    }
  }

  const guide = PURPOSE_GUIDES[certInput.profile];
  const current = categories.find((item) => item.id === category) ?? categories[0];
  if (!current || !prefsReady) {
    return (
      <div className="generator-layout">
        <section className="panel generator-panel">
          <p className="section-copy">{current ? "Loading your generator settings…" : "No generator parts are enabled for your account."}</p>
        </section>
      </div>
    );
  }
  const certReady =
    certInput.commonName.trim() !== "" &&
    certInput.pfxPassword.length >= 8 &&
    (guide.email !== "required" || certInput.email.trim() !== "");
  return (
    <div className="generator-layout">
      <div className="section-nav-strip" role="tablist" aria-label="Generator categories">
        {categories.map((item) => (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={item.id === category}
            className={`section-nav-button ${item.id === category ? "active" : ""}`}
            onClick={() => setCategory(item.id)}
          >
            {item.label}
          </button>
        ))}
      </div>

      <section className="panel generator-panel">
        <div className="panel-header">
          <div>
            <p className="eyebrow">Generator</p>
            <h2>{current.label}</h2>
            <p className="section-copy">{current.copy}</p>
          </div>
          {category === "passwords" ? (
            <Segmented
              label="Password type"
              value={passwordKind}
              onChange={setPasswordKind}
              options={[
                { value: "password", label: "Password" },
                { value: "passphrase", label: "Passphrase" }
              ]}
            />
          ) : null}
          {category === "keys" ? (
            <Segmented
              label="Key type"
              value={keyKind}
              onChange={setKeyKind}
              options={[
                { value: "key", label: "Random key" },
                { value: "token", label: "API token" },
                { value: "uuid", label: "UUID" }
              ]}
            />
          ) : null}
          {category === "keypairs" ? (
            <Segmented
              label="Keypair type"
              value={pairKind}
              onChange={setPairKind}
              options={[
                { value: "rsa", label: "RSA" },
                { value: "ec", label: "EC" },
                { value: "ssh", label: "SSH (Ed25519)" }
              ]}
            />
          ) : null}
        </div>

        {category === "passwords" && passwordKind === "password" ? (
          <>
            <div className="form-grid">
              <RangeWithNumber label="Length (characters)" value={passwordOptions.length} min={8} max={64} onChange={(length) => setPasswordOptions((c) => ({ ...c, length }))} />
              <div className="generator-checks">
                {(
                  [
                    ["lowercase", "Lowercase a-z"],
                    ["uppercase", "Uppercase A-Z"],
                    ["digits", "Digits 0-9"],
                    ["symbols", "Symbols"],
                    ["avoidAmbiguous", "Avoid look-alikes (l 1 I O 0)"],
                    ["safeSymbols", "Only shell- and URL-safe symbols"]
                  ] as [keyof PasswordOptions, string][]
                ).map(([key, label]) => (
                  <label key={key} className="checkbox">
                    <input type="checkbox" checked={Boolean(passwordOptions[key])} onChange={(event) => setPasswordOptions((c) => ({ ...c, [key]: event.target.checked }))} />
                    <span>{label}</span>
                  </label>
                ))}
              </div>
            </div>
            <GeneratedValue value={value} onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            <p className="generator-meta">≈ {passwordEntropyBits(passwordOptions)} bits of entropy</p>
            <div className="action-row">
              <button type="button" className="button primary" onClick={regenerateAndRemember}>
                Regenerate
              </button>
              <CopyButton value={value} label="Copy password" onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            </div>
          </>
        ) : null}

        {category === "passwords" && passwordKind === "passphrase" ? (
          <>
            <div className="form-grid">
              <RangeWithNumber label="Words" value={passphraseOptions.words} min={3} max={10} wide={false} onChange={(words) => setPassphraseOptions((c) => ({ ...c, words }))} />
              <label>
                <span>Separator</span>
                <Picker id="separator" openId={openPicker} setOpenId={setOpenPicker} value={passphraseOptions.separator} options={SEPARATOR_OPTIONS} onSelect={(separator) => setPassphraseOptions((c) => ({ ...c, separator }))} />
              </label>
              <div className="generator-checks">
                <label className="checkbox">
                  <input type="checkbox" checked={passphraseOptions.capitalize} onChange={(event) => setPassphraseOptions((c) => ({ ...c, capitalize: event.target.checked }))} />
                  <span>Capitalise words</span>
                </label>
                <label className="checkbox">
                  <input type="checkbox" checked={passphraseOptions.addDigit} onChange={(event) => setPassphraseOptions((c) => ({ ...c, addDigit: event.target.checked }))} />
                  <span>Add a digit</span>
                </label>
              </div>
            </div>
            <GeneratedValue value={value} onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            <p className="generator-meta">≈ {passphraseEntropyBits(passphraseOptions)} bits of entropy</p>
            <div className="action-row">
              <button type="button" className="button primary" onClick={regenerateAndRemember}>
                Regenerate
              </button>
              <CopyButton value={value} label="Copy passphrase" onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            </div>
          </>
        ) : null}

        {category === "keys" && keyKind === "key" ? (
          <>
            <div className="form-grid">
              <label>
                <span>Key size</span>
                <Picker id="bytes" openId={openPicker} setOpenId={setOpenPicker} value={String(keyBytes)} options={BYTE_OPTIONS} onSelect={(next) => setKeyBytes(Number(next))} />
              </label>
              <label>
                <span>Encoding</span>
                <Picker id="encoding" openId={openPicker} setOpenId={setOpenPicker} value={keyEncoding} options={ENCODING_OPTIONS} onSelect={setKeyEncoding} />
              </label>
            </div>
            <GeneratedValue value={value} onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            <p className="generator-meta">
              {value.length} characters · {keyBytes * 8} bits
            </p>
            <div className="action-row">
              <button type="button" className="button primary" onClick={regenerateAndRemember}>
                Regenerate
              </button>
              <CopyButton value={value} label="Copy key" onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            </div>
          </>
        ) : null}

        {category === "keys" && keyKind === "token" ? (
          <>
            <div className="form-grid">
              <label>
                <span>Prefix (optional)</span>
                <input value={tokenPrefix} placeholder="ak_" onChange={(event) => setTokenPrefix(event.target.value)} />
              </label>
              <RangeWithNumber label="Length (characters)" value={tokenLength} min={16} max={96} wide={false} onChange={setTokenLength} />
            </div>
            <GeneratedValue value={value} onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            <p className="generator-meta">Letters and digits only — safe in URLs, headers and environment files.</p>
            <div className="action-row">
              <button type="button" className="button primary" onClick={regenerateAndRemember}>
                Regenerate
              </button>
              <CopyButton value={value} label="Copy token" onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            </div>
          </>
        ) : null}

        {category === "keys" && keyKind === "uuid" ? (
          <>
            <GeneratedValue value={value} onCopied={(text) => { onMessage(text); rememberCurrentSimple(); }} />
            <p className="generator-meta">Version 4 random identifier.</p>
            <div className="action-row">
              <button type="button" className="button primary" onClick={regenerateAndRemember}>
                Regenerate
              </button>
              <CopyButton value={value} label="Copy UUID" onCopied={onMessage} />
            </div>
          </>
        ) : null}

        {category === "keypairs" ? (
          <>
            <div className="form-grid">
              {pairKind === "rsa" ? (
                <label>
                  <span>Key size</span>
                  <Picker id="rsa" openId={openPicker} setOpenId={setOpenPicker} value={String(rsaBits)} options={RSA_OPTIONS} onSelect={(next) => setRsaBits(Number(next) as 2048 | 3072 | 4096)} />
                </label>
              ) : null}
              {pairKind === "ec" ? (
                <label>
                  <span>Curve</span>
                  <Picker id="curve" openId={openPicker} setOpenId={setOpenPicker} value={ecCurve} options={CURVE_OPTIONS} onSelect={setEcCurve} />
                </label>
              ) : null}
              {pairKind === "ssh" ? (
                <label className="wide">
                  <span>Comment (shown in the public key line)</span>
                  <input value={sshComment} placeholder="deploy@example" onChange={(event) => setSshComment(event.target.value)} />
                </label>
              ) : null}
            </div>
            <p className="generator-meta">
              {pairKind === "rsa" ? "PKCS#8 private key and SPKI public key in PEM — JWT RS256 signing keys, general purpose." : null}
              {pairKind === "ec" ? "PKCS#8 private key and SPKI public key in PEM — JWT ES256/ES384, modern TLS." : null}
              {pairKind === "ssh" ? "OpenSSH private key file plus the authorized_keys line — servers, Git deploy keys." : null}
            </p>
            {pairKind === "ssh" && sshAvailable === false ? <div className="banner compact">This browser cannot generate Ed25519 keys. Use a current Edge, Chrome or Firefox.</div> : null}
            <div className="action-row">
              <button type="button" className="button primary" disabled={working || busy || (pairKind === "ssh" && sshAvailable === false)} onClick={() => void generatePair()}>
                {working ? "Generating…" : pairOutput ? "Generate another" : "Generate keypair"}
              </button>
            </div>
            {pairOutput ? (
              <>
                <OutputBlock title={pairKind === "ssh" ? "Private key (OpenSSH)" : "Private key (PKCS#8 PEM)"} value={pairOutput.private} fileName={pairOutput.privateName} onCopied={onMessage} />
                <OutputBlock title={pairKind === "ssh" ? "Public key (authorized_keys line)" : "Public key (SPKI PEM)"} value={pairOutput.public} fileName={pairOutput.publicName} onCopied={onMessage} />
              </>
            ) : null}
          </>
        ) : null}

        {category === "certificates" ? (
          <>
            <div className="form-grid">
              <label className="wide">
                <span>Purpose — written into the certificate as its intended usage</span>
                <Picker
                  id="profile"
                  openId={openPicker}
                  setOpenId={setOpenPicker}
                  value={certInput.profile}
                  options={PROFILE_OPTIONS}
                  onSelect={(profile) => {
                    setCertInput((c) => ({ ...c, profile }));
                    setCertResult(null);
                  }}
                />
              </label>
            </div>
            <p className="generator-meta">{guide.intro}</p>
            <div className="form-grid">
              <label className={guide.showSans ? undefined : "wide"}>
                <span>{guide.cnLabel}</span>
                <input value={certInput.commonName} placeholder={guide.cnPlaceholder} onChange={(event) => setCertInput((c) => ({ ...c, commonName: event.target.value }))} />
              </label>
              {guide.showSans ? (
                <label>
                  <span>Other host names / IP addresses (comma or space separated)</span>
                  <input value={certDnsNames} placeholder="api.example.internal, 10.0.0.5" onChange={(event) => setCertDnsNames(event.target.value)} />
                </label>
              ) : null}
              {guide.email !== "hidden" ? (
                <label>
                  <span>{guide.emailLabel}</span>
                  <input value={certInput.email} placeholder="name@example.internal" onChange={(event) => setCertInput((c) => ({ ...c, email: event.target.value }))} />
                </label>
              ) : null}
              {guide.showOrganization ? (
                <label>
                  <span>Organisation</span>
                  <input value={certInput.organization} placeholder="Example Org" onChange={(event) => setCertInput((c) => ({ ...c, organization: event.target.value }))} />
                </label>
              ) : null}
              {guide.showUnit ? (
                <label>
                  <span>Department (optional)</span>
                  <input value={certInput.organizationalUnit} placeholder="Finance" onChange={(event) => setCertInput((c) => ({ ...c, organizationalUnit: event.target.value }))} />
                </label>
              ) : null}
              {guide.showCountry ? (
                <label>
                  <span>Country (optional, two-letter code)</span>
                  <input value={certInput.country} placeholder="CZ" maxLength={2} onChange={(event) => setCertInput((c) => ({ ...c, country: event.target.value.toUpperCase() }))} />
                </label>
              ) : null}
            </div>
            <p className="generator-meta">Key and packaging — the defaults suit most testing.</p>
            <div className="form-grid">
              <label>
                <span>Key</span>
                <Picker id="algo" openId={openPicker} setOpenId={setOpenPicker} value={certInput.keyAlgorithm} options={KEY_ALGO_OPTIONS} onSelect={(keyAlgorithm) => setCertInput((c) => ({ ...c, keyAlgorithm }))} />
              </label>
              <label>
                <span>Valid for (days)</span>
                <input type="number" min={1} max={3650} value={certInput.validityDays} onChange={(event) => setCertInput((c) => ({ ...c, validityDays: Number(event.target.value) }))} />
              </label>
              <label>
                <span>PFX format</span>
                <Picker id="pfx" openId={openPicker} setOpenId={setOpenPicker} value={certInput.pfxEncoding} options={PFX_OPTIONS} onSelect={(pfxEncoding) => setCertInput((c) => ({ ...c, pfxEncoding }))} />
              </label>
              <label className="wide">
                <span>PFX password (at least 8 characters)</span>
                <div className="password-field-row">
                  <input value={certInput.pfxPassword} autoComplete="new-password" spellCheck={false} onChange={(event) => setCertInput((c) => ({ ...c, pfxPassword: event.target.value }))} />
                  <button type="button" className="button ghost compact-button" onClick={() => setCertInput((c) => ({ ...c, pfxPassword: generatePassword({ ...defaultPasswordOptions, length: 20, symbols: false }) }))}>
                    Generate
                  </button>
                </div>
              </label>
            </div>
            {certError ? <div className="banner compact">{certError}</div> : null}
            <div className="action-row">
              <button type="button" className="button primary" disabled={!certReady || working || busy} onClick={() => void generateCertificate()}>
                {working ? "Generating…" : "Generate certificate"}
              </button>
            </div>
            {certResult ? (
              <>
                <dl className="detail-grid">
                  <div>
                    <dt>Subject</dt>
                    <dd>{certResult.subject}</dd>
                  </div>
                  <div>
                    <dt>Valid until</dt>
                    <dd>{new Date(certResult.notAfter).toLocaleDateString()}</dd>
                  </div>
                  <div>
                    <dt>Intended purposes</dt>
                    <dd>{[...certResult.extendedKeyUsages, ...certResult.keyUsages].join(" · ")}</dd>
                  </div>
                  <div>
                    <dt>Thumbprint (SHA-1)</dt>
                    <dd className="mono">{certResult.thumbprintSha1}</dd>
                  </div>
                  <div>
                    <dt>Thumbprint (SHA-256)</dt>
                    <dd className="mono">{certResult.thumbprintSha256}</dd>
                  </div>
                </dl>
                <div className="action-row">
                  <button type="button" className="button primary" onClick={() => download(certResult.pfxFileName, fromBase64(certResult.pfxBase64), "application/x-pkcs12")}>
                    Download PFX
                  </button>
                  <CopyButton value={certInput.pfxPassword} label="Copy PFX password" onCopied={onMessage} />
                </div>
                <OutputBlock title="Certificate (PEM)" value={certResult.certificatePem} fileName={certResult.pfxFileName.replace(/\.pfx$/, ".crt")} onCopied={onMessage} />
                <OutputBlock title="Private key (PKCS#8 PEM) — keep this safe" value={certResult.privateKeyPem} fileName={certResult.pfxFileName.replace(/\.pfx$/, ".key")} onCopied={onMessage} />
              </>
            ) : null}
          </>
        ) : null}
      </section>
    </div>
  );
}
