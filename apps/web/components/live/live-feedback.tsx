"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { isNotFound } from "../../lib/live/api";

export function useReplaceNotFound(error: unknown): boolean {
  const router = useRouter();
  const notFound = isNotFound(error);
  useEffect(() => {
    if (notFound) router.replace("/live/not-found");
  }, [notFound, router]);
  return notFound;
}
