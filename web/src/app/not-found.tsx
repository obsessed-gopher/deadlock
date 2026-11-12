import { ErrorState } from "@/components/ErrorState.tsx";
import { getI18n } from "@/i18n/server.ts";

export default async function NotFound() {
  return <ErrorState i18n={await getI18n()} kind="notFound" />;
}
