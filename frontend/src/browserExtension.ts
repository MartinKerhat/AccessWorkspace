export function browserExtensionStatusLabel(status: string) {
  switch (status) {
    case "available":
      return "ready";
    case "preview":
      return "preview";
    case "planned":
      return "planned";
    case "pending_store":
      return "store setup";
    case "signing_required":
      return "signing needed";
    default:
      return status;
  }
}
