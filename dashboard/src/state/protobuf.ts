// dashboard/src/state/protobuf.ts
//
// The generated _pb.ts files carry `@ts-nocheck`, which strips the inherited
// Message<> method types (toBinary/fromBinary) from exported classes. We
// concentrate the structural casts here so views and hooks can stay clean.

/** Minimal shape of a buf-es Message class with a static fromBinary. */
type PbClass<T> = {
  new (data?: unknown): T;
  fromBinary(bytes: Uint8Array): T;
};

/** Minimal runtime surface used to serialize an instance. */
type PbInstance = { toBinary(): Uint8Array };

/** Decode raw bytes into a generated message of type T. */
export function pbFromBinary<T>(cls: unknown, bytes: Uint8Array): T {
  return (cls as PbClass<T>).fromBinary(bytes);
}

/** Encode an instance of a generated message into raw bytes. */
export function pbToBinary(msg: unknown): Uint8Array {
  return (msg as PbInstance).toBinary();
}

/** Minimal runtime surface used to convert an instance to a JSON value. */
type PbJsonable = { toJson(options?: unknown): unknown };

/** Convert a generated message instance into a plain JSON value. */
export function pbToJson(msg: unknown, options?: { emitDefaultValues?: boolean }): unknown {
  return (msg as PbJsonable).toJson(options);
}
