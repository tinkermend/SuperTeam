import type { Tone } from "@/components/superteam";

/** Shared fire status → tone for table dots, rail, and detail. */
export function automationFireTone(status: string): Tone {
  switch (status) {
    case "succeeded":
      return "ok";
    case "failed":
      return "danger";
    case "skipped_overlap":
    case "skipped_disabled":
      return "warn";
    case "pending":
      return "info";
    default:
      return "mute";
  }
}
