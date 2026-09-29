import type { FileSearchResult, SearchMatch, SearchOptions } from "../../bindings/veda/models";

export type FindModes = {
  caseSensitive: boolean;
  wholeWord: boolean;
  useRegex: boolean;
};

export const EMPTY_FIND_MODES: FindModes = {
  caseSensitive: false,
  wholeWord: false,
  useRegex: false,
};

export type RevealTarget = {
  path: string;
  match: SearchMatch;
  nonce: number;
};

export type WorkspaceSearchSession = {
  query: string;
  modes: FindModes;
  files: FileSearchResult[];
  error: string;
  truncated: boolean;
  active: number;
  searched: boolean;
};

export const EMPTY_WORKSPACE_SEARCH: WorkspaceSearchSession = {
  query: "",
  modes: { ...EMPTY_FIND_MODES },
  files: [],
  error: "",
  truncated: false,
  active: 0,
  searched: false,
};

export function searchOptions(modes: FindModes, extra?: Partial<SearchOptions>): SearchOptions {
  return {
    caseSensitive: modes.caseSensitive,
    wholeWord: modes.wholeWord,
    useRegex: modes.useRegex,
    maxMatches: extra?.maxMatches ?? 2000,
    maxPerFile: extra?.maxPerFile ?? 200,
    contextLines: extra?.contextLines ?? 0,
  };
}

export function utf16FromUtf8Offset(text: string, byteOffset: number): number {
  if (byteOffset <= 0) {
    return 0;
  }
  const encoder = new TextEncoder();
  let bytes = 0;
  let i = 0;
  while (i < text.length) {
    if (bytes >= byteOffset) {
      return i;
    }
    const code = text.codePointAt(i) ?? 0;
    const ch = String.fromCodePoint(code);
    bytes += encoder.encode(ch).byteLength;
    i += code > 0xffff ? 2 : 1;
  }
  return text.length;
}

export function fillTemplate(template: string, vars: Record<string, string | number>): string {
  return template.replace(/\{(\w+)\}/g, (_, key: string) => String(vars[key] ?? ""));
}
