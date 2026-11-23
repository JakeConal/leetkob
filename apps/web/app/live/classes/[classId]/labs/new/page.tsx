"use client";

import { useParams } from "next/navigation";

import { CreateLabForm } from "../../../../../../components/live/create-lab-form";

export default function LiveCreateLabPage() {
  const params = useParams<{ classId: string }>();
  const classId = typeof params.classId === "string" ? params.classId : "";
  if (classId.length === 0) return <p className="live-muted">This class address is not valid.</p>;
  return <CreateLabForm classId={classId} />;
}
