import type { ByteEncoding } from "../generator/random";

export type PickerOption<T extends string> = { value: T; label: string; description?: string };

export const BYTE_OPTIONS: PickerOption<string>[] = [
  { value: "16", label: "16 bytes (128 bits)", description: "AES-128" },
  { value: "24", label: "24 bytes (192 bits)", description: "AES-192" },
  { value: "32", label: "32 bytes (256 bits)", description: "AES-256, HMAC-SHA256, Fernet — 64 hex characters" },
  { value: "48", label: "48 bytes (384 bits)", description: "HMAC-SHA384" },
  { value: "64", label: "64 bytes (512 bits)", description: "HMAC-SHA512, JWT HS512 secrets" },
  { value: "128", label: "128 bytes (1024 bits)", description: "Libraries that insist on very long secrets" }
];

export const ENCODING_OPTIONS: PickerOption<ByteEncoding>[] = [
  { value: "hex", label: "Hex", description: "0-9 a-f, two characters per byte" },
  { value: "base64", label: "Base64", description: "Standard alphabet with padding" },
  { value: "base64url", label: "Base64url", description: "URL-safe alphabet, no padding — Fernet, JWT secrets" }
];

// Single-value picker in the app's house style (same markup and classes as
// the object form), with one open picker at a time per screen. Shared by the
// Generator page and the Key Vault value modal.
export function Picker<T extends string>({
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
