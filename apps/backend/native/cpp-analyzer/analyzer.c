/*
 * Native C++ declaration resolver for the checker image.
 * Go consumes the JSON this program writes. There is no CGO boundary.
 *
 * Locations are lines in the submitted file. A comment or string is not a
 * reference. Unsupported input is status "unsure" with no references, so the
 * consumer cannot invent a violation.
 */

#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef void *CXIndex;
typedef void *CXTranslationUnit;
typedef void *CXFile;
typedef void *CXDiagnostic;
typedef void *CXClientData;

typedef struct {
  const void *data;
  unsigned private_flags;
} CXString;

typedef struct {
  const void *ptr_data[2];
  unsigned int_data;
} CXSourceLocation;

typedef struct {
  int kind;
  int xdata;
  const void *data[3];
} CXCursor;

typedef enum {
  CXChildVisit_Break = 0,
  CXChildVisit_Continue = 1,
  CXChildVisit_Recurse = 2
} CXChildVisitResult;

typedef CXChildVisitResult (*CXCursorVisitor)(CXCursor cursor, CXCursor parent, CXClientData data);
typedef void (*CXInclusionVisitor)(CXFile included_file, CXSourceLocation *inclusion_stack, unsigned include_len,
                                   CXClientData client_data);

enum {
  CK_Struct = 2,
  CK_Class = 4,
  CK_Function = 8,
  CK_CXXMethod = 21,
  CK_Namespace = 22,
  CK_Constructor = 24,
  CK_Destructor = 25,
  CK_Conversion = 26,
  CK_FunctionTemplate = 30,
  CK_ClassTemplate = 31,
  CK_Overloaded = 49,
  CK_DeclRef = 101,
  CK_MemberRef = 102,
  CK_Call = 103,
  CK_Using = 35,
  CK_Inclusion = 503,
  CK_MacroDef = 501,
  CK_TU = 350
};

CXIndex clang_createIndex(int excludeDeclarationsFromPCH, int displayDiagnostics);
void clang_disposeIndex(CXIndex index);
CXTranslationUnit clang_parseTranslationUnit(CXIndex index, const char *source_filename,
                                             const char *const *command_line_args, int num_command_line_args,
                                             void *unsaved_files, unsigned num_unsaved_files, unsigned options);
void clang_disposeTranslationUnit(CXTranslationUnit unit);
unsigned clang_getNumDiagnostics(CXTranslationUnit unit);
CXDiagnostic clang_getDiagnostic(CXTranslationUnit unit, unsigned index);
int clang_getDiagnosticSeverity(CXDiagnostic diagnostic);
void clang_disposeDiagnostic(CXDiagnostic diagnostic);
CXCursor clang_getTranslationUnitCursor(CXTranslationUnit unit);
unsigned clang_visitChildren(CXCursor parent, CXCursorVisitor visitor, CXClientData data);
int clang_getCursorKind(CXCursor cursor);
CXString clang_getCursorSpelling(CXCursor cursor);
const char *clang_getCString(CXString string);
void clang_disposeString(CXString string);
CXCursor clang_getCursorReferenced(CXCursor cursor);
int clang_Cursor_isNull(CXCursor cursor);
CXCursor clang_getCursorSemanticParent(CXCursor cursor);
CXCursor clang_getSpecializedCursorTemplate(CXCursor cursor);
CXCursor clang_getCanonicalCursor(CXCursor cursor);
int clang_Cursor_isInlineNamespace(CXCursor cursor);
unsigned clang_getNumOverloadedDecls(CXCursor cursor);
CXCursor clang_getOverloadedDecl(CXCursor cursor, unsigned index);
CXSourceLocation clang_getCursorLocation(CXCursor cursor);
void clang_getFileLocation(CXSourceLocation location, CXFile *file, unsigned *line, unsigned *column,
                           unsigned *offset);
void clang_getSpellingLocation(CXSourceLocation location, CXFile *file, unsigned *line, unsigned *column,
                               unsigned *offset);
void clang_getExpansionLocation(CXSourceLocation location, CXFile *file, unsigned *line, unsigned *column,
                                unsigned *offset);
CXString clang_getFileName(CXFile file);
CXFile clang_getFile(CXTranslationUnit unit, const char *file_name);
void clang_getInclusions(CXTranslationUnit unit, CXInclusionVisitor visitor, CXClientData data);

enum { MAX_REFS = 4096, MAX_SOURCE = 65536, MAX_EDGES = 8192, MAX_NAME = 256, MAX_QUAL = 512 };

typedef struct {
  char kind[16];
  char name[MAX_NAME];
  char qualified[MAX_QUAL];
  char header[MAX_NAME];
  int resolved;
  int user_declared;
  unsigned line;
  unsigned column;
} Ref;

typedef struct {
  CXFile file;
  CXFile includer;
} Edge;

typedef struct {
  CXFile file;
  char header[MAX_NAME];
} HeaderEnt;

static Ref g_refs[MAX_REFS];
static int g_nrefs;
static int g_overflow;
static Edge g_edges[MAX_EDGES];
static int g_nedges;
static HeaderEnt g_headers[MAX_EDGES];
static int g_nheaders;
static CXFile g_main;
static char g_main_name[PATH_MAX];

static const char *cstr(CXString value) {
  const char *text = clang_getCString(value);
  return text ? text : "";
}

static void copy_str(char *dest, size_t size, const char *src) {
  size_t i = 0;
  if (size == 0) {
    return;
  }
  if (!src) {
    src = "";
  }
  for (; src[i] != '\0' && i + 1 < size; i++) {
    dest[i] = src[i];
  }
  dest[i] = '\0';
}

static const char *base_name(const char *path) {
  const char *slash = strrchr(path, '/');
  return slash ? slash + 1 : path;
}

static int file_is_main(CXFile file) {
  CXString name;
  int same;
  if (!file) {
    return 0;
  }
  if (g_main && file == g_main) {
    return 1;
  }
  name = clang_getFileName(file);
  same = g_main_name[0] != '\0' && strcmp(cstr(name), g_main_name) == 0;
  clang_disposeString(name);
  return same;
}

static void remember_main(CXFile file) {
  CXString name;
  if (!file) {
    return;
  }
  g_main = file;
  name = clang_getFileName(file);
  copy_str(g_main_name, sizeof g_main_name, cstr(name));
  clang_disposeString(name);
}

static CXFile find_main(CXTranslationUnit unit, const char *path) {
  char real[PATH_MAX];
  CXFile file;
  if (realpath(path, real)) {
    file = clang_getFile(unit, real);
    if (file) {
      remember_main(file);
      return file;
    }
  }
  file = clang_getFile(unit, path);
  if (file) {
    remember_main(file);
  }
  return file;
}

static const char *header_of(CXFile file) {
  int i;
  for (i = 0; i < g_nheaders; i++) {
    if (g_headers[i].file == file) {
      return g_headers[i].header;
    }
  }
  return "";
}

static void add_header(CXFile file, const char *header) {
  if (!file || !header || header[0] == '\0' || header_of(file)[0] != '\0' || g_nheaders >= MAX_EDGES) {
    return;
  }
  g_headers[g_nheaders].file = file;
  copy_str(g_headers[g_nheaders].header, MAX_NAME, header);
  g_nheaders++;
}

static void inclusion_visitor(CXFile included, CXSourceLocation *stack, unsigned include_len, CXClientData data) {
  CXFile includer = NULL;
  unsigned line = 0;
  unsigned column = 0;
  unsigned offset = 0;
  (void)data;
  if (include_len == 0 || g_nedges >= MAX_EDGES) {
    return;
  }
  clang_getFileLocation(stack[0], &includer, &line, &column, &offset);
  g_edges[g_nedges].file = included;
  g_edges[g_nedges].includer = includer;
  g_nedges++;
}

static void resolve_headers(void) {
  int changed = 1;
  while (changed) {
    int i;
    changed = 0;
    for (i = 0; i < g_nedges; i++) {
      const char *parent;
      CXString name;
      if (header_of(g_edges[i].file)[0] != '\0') {
        continue;
      }
      if (file_is_main(g_edges[i].includer)) {
        name = clang_getFileName(g_edges[i].file);
        add_header(g_edges[i].file, base_name(cstr(name)));
        clang_disposeString(name);
        changed = 1;
        continue;
      }
      parent = header_of(g_edges[i].includer);
      if (parent[0] != '\0') {
        add_header(g_edges[i].file, parent);
        changed = 1;
      }
    }
  }
}

static int has_error(CXTranslationUnit unit) {
  unsigned count = clang_getNumDiagnostics(unit);
  unsigned i;
  for (i = 0; i < count; i++) {
    CXDiagnostic diagnostic = clang_getDiagnostic(unit, i);
    int severity = clang_getDiagnosticSeverity(diagnostic);
    clang_disposeDiagnostic(diagnostic);
    if (severity >= 3) {
      return 1;
    }
  }
  return 0;
}

static int named_scope(int kind) {
  return kind == CK_Namespace || kind == CK_Class || kind == CK_Struct || kind == CK_ClassTemplate ||
         kind == CK_Function || kind == CK_FunctionTemplate || kind == CK_CXXMethod || kind == CK_Constructor ||
         kind == CK_Destructor || kind == CK_Conversion;
}

static int functionish(int kind) {
  return kind == CK_Function || kind == CK_FunctionTemplate || kind == CK_CXXMethod || kind == CK_Constructor ||
         kind == CK_Destructor || kind == CK_Conversion;
}

static void append_text(char *dest, size_t size, const char *src) {
  size_t used = strlen(dest);
  size_t i = 0;
  if (used >= size) {
    return;
  }
  for (; src[i] != '\0' && used + 1 < size; i++, used++) {
    dest[used] = src[i];
  }
  dest[used] = '\0';
}

static int qualify(CXCursor decl, char *qualified, size_t qualified_size, int *user_declared, char *header,
                   size_t header_size) {
  CXCursor specialized = clang_getSpecializedCursorTemplate(decl);
  CXCursor chain[24];
  CXFile file = NULL;
  unsigned line = 0;
  unsigned column = 0;
  unsigned offset = 0;
  int count = 0;
  int i;
  if (!clang_Cursor_isNull(specialized) && clang_getCursorKind(specialized) != 0) {
    decl = specialized;
  }
  decl = clang_getCanonicalCursor(decl);
  clang_getFileLocation(clang_getCursorLocation(decl), &file, &line, &column, &offset);
  *user_declared = file_is_main(file);
  if (*user_declared) {
    copy_str(header, header_size, "");
  } else {
    copy_str(header, header_size, header_of(file));
  }
  while (!clang_Cursor_isNull(decl) && count < 24) {
    int kind = clang_getCursorKind(decl);
    CXString spelling;
    if (kind == CK_TU || kind == 0) {
      break;
    }
    if (kind == CK_Namespace && clang_Cursor_isInlineNamespace(decl)) {
      decl = clang_getCursorSemanticParent(decl);
      continue;
    }
    if (named_scope(kind)) {
      spelling = clang_getCursorSpelling(decl);
      if (cstr(spelling)[0] != '\0') {
        chain[count++] = decl;
      }
      clang_disposeString(spelling);
    }
    decl = clang_getCursorSemanticParent(decl);
  }
  qualified[0] = '\0';
  for (i = count - 1; i >= 0; i--) {
    CXString spelling = clang_getCursorSpelling(chain[i]);
    if (qualified[0] != '\0') {
      append_text(qualified, qualified_size, "::");
    }
    append_text(qualified, qualified_size, cstr(spelling));
    clang_disposeString(spelling);
  }
  return qualified[0] != '\0';
}

static const char *leaf_name(const char *qualified) {
  const char *last = qualified;
  const char *cursor = qualified;
  while (cursor[0] != '\0' && cursor[1] != '\0') {
    if (cursor[0] == ':' && cursor[1] == ':') {
      last = cursor + 2;
      cursor++;
    }
    cursor++;
  }
  return last;
}

static void add_ref(const char *kind, const char *name, const char *qualified, const char *header, int resolved,
                    int user_declared, unsigned line, unsigned column) {
  int i;
  Ref *ref;
  if (line == 0) {
    return;
  }
  if (g_nrefs >= MAX_REFS) {
    g_overflow = 1;
    return;
  }
  for (i = 0; i < g_nrefs; i++) {
    if (g_refs[i].line == line && strcmp(g_refs[i].kind, kind) == 0 && strcmp(g_refs[i].name, name) == 0 &&
        strcmp(g_refs[i].qualified, qualified) == 0) {
      return;
    }
  }
  ref = &g_refs[g_nrefs++];
  copy_str(ref->kind, sizeof ref->kind, kind);
  copy_str(ref->name, sizeof ref->name, name);
  copy_str(ref->qualified, sizeof ref->qualified, qualified);
  copy_str(ref->header, sizeof ref->header, header ? header : "");
  ref->resolved = resolved;
  ref->user_declared = user_declared;
  ref->line = line;
  ref->column = column == 0 ? 1 : column;
}

static void emit_decl(const char *kind, CXCursor decl, unsigned line, unsigned column) {
  char qualified[MAX_QUAL];
  char header[MAX_NAME];
  int user_declared = 0;
  if (!functionish(clang_getCursorKind(decl))) {
    return;
  }
  if (!qualify(decl, qualified, sizeof qualified, &user_declared, header, sizeof header)) {
    return;
  }
  add_ref(kind, leaf_name(qualified), qualified, header, 1, user_declared, line, column);
}

static void emit_target(const char *kind, CXCursor target, unsigned line, unsigned column, const char *fallback,
                        int depth) {
  int kind_id = clang_getCursorKind(target);
  if (depth > 4 || clang_Cursor_isNull(target)) {
    if (fallback && fallback[0] != '\0') {
      add_ref(kind, fallback, "", "", 0, 0, line, column);
    }
    return;
  }
  if (kind_id == CK_Overloaded) {
    unsigned count = clang_getNumOverloadedDecls(target);
    unsigned i;
    int emitted = 0;
    for (i = 0; i < count; i++) {
      CXCursor overloaded = clang_getOverloadedDecl(target, i);
      if (functionish(clang_getCursorKind(overloaded))) {
        emit_decl(kind, overloaded, line, column);
        emitted = 1;
      }
    }
    if (!emitted && fallback && fallback[0] != '\0') {
      add_ref(kind, fallback, "", "", 0, 0, line, column);
    }
    return;
  }
  if (functionish(kind_id)) {
    emit_decl(kind, target, line, column);
    return;
  }
  emit_target(kind, clang_getCursorReferenced(target), line, column, fallback, depth + 1);
}

static int location_in_main(CXCursor cursor, unsigned *line, unsigned *column) {
  CXSourceLocation location = clang_getCursorLocation(cursor);
  CXFile file = NULL;
  unsigned offset = 0;
  unsigned spelling_line = 0;
  unsigned spelling_column = 0;
  unsigned expansion_line = 0;
  unsigned expansion_column = 0;
  clang_getFileLocation(location, &file, line, column, &offset);
  if (file_is_main(file) && *line > 0) {
    return 1;
  }
  file = NULL;
  clang_getSpellingLocation(location, &file, &spelling_line, &spelling_column, &offset);
  if (file_is_main(file) && spelling_line > 0) {
    *line = spelling_line;
    *column = spelling_column;
    return 1;
  }
  file = NULL;
  clang_getExpansionLocation(location, &file, &expansion_line, &expansion_column, &offset);
  if (file_is_main(file) && expansion_line > 0) {
    *line = expansion_line;
    *column = expansion_column;
    return 1;
  }
  return 0;
}

static CXChildVisitResult visit(CXCursor cursor, CXCursor parent, CXClientData client) {
  int depth = *(int *)client;
  int kind = clang_getCursorKind(cursor);
  unsigned line = 0;
  unsigned column = 0;
  const char *out_kind = NULL;
  CXString spelling;
  char fallback[MAX_NAME];
  (void)parent;
  if (kind != CK_MacroDef && depth < 64) {
    int next = depth + 1;
    clang_visitChildren(cursor, visit, &next);
  }
  if (!location_in_main(cursor, &line, &column)) {
    return CXChildVisit_Continue;
  }
  if (kind == CK_Inclusion) {
    spelling = clang_getCursorSpelling(cursor);
    add_ref("include", cstr(spelling), "", cstr(spelling), 1, 0, line, column);
    clang_disposeString(spelling);
    return CXChildVisit_Continue;
  }
  if (kind == CK_Call) {
    out_kind = "call";
  } else if (kind == CK_Using) {
    out_kind = "using";
  } else if (kind == CK_Overloaded || kind == CK_DeclRef || kind == CK_MemberRef) {
    out_kind = "reference";
  } else {
    return CXChildVisit_Continue;
  }
  spelling = clang_getCursorSpelling(cursor);
  copy_str(fallback, sizeof fallback, cstr(spelling));
  clang_disposeString(spelling);
  if (kind == CK_Overloaded) {
    emit_target(out_kind, cursor, line, column, fallback, 0);
  } else {
    emit_target(out_kind, clang_getCursorReferenced(cursor), line, column, fallback, 0);
  }
  return CXChildVisit_Continue;
}

static void json_escape(FILE *out, const char *text) {
  const unsigned char *cursor = (const unsigned char *)(text ? text : "");
  fputc('"', out);
  for (; *cursor != '\0'; cursor++) {
    unsigned char ch = *cursor;
    if (ch == '"' || ch == '\\') {
      fprintf(out, "\\%c", ch);
    } else if (ch < 0x20) {
      fprintf(out, "\\u%04x", ch);
    } else {
      fputc(ch, out);
    }
  }
  fputc('"', out);
}

static void emit_unsure(const char *reason) {
  fputs("{\"version\":1,\"status\":\"unsure\",\"reason\":", stdout);
  json_escape(stdout, reason);
  fputs(",\"references\":[]}\n", stdout);
}

static void emit_ok(void) {
  int i;
  fputs("{\"version\":1,\"status\":\"ok\",\"references\":[", stdout);
  for (i = 0; i < g_nrefs; i++) {
    if (i > 0) {
      fputc(',', stdout);
    }
    fputs("{\"kind\":", stdout);
    json_escape(stdout, g_refs[i].kind);
    fputs(",\"name\":", stdout);
    json_escape(stdout, g_refs[i].name);
    fputs(",\"qualifiedName\":", stdout);
    json_escape(stdout, g_refs[i].qualified);
    fputs(",\"header\":", stdout);
    json_escape(stdout, g_refs[i].header);
    fprintf(stdout, ",\"resolved\":%s,\"userDeclared\":%s,\"line\":%u,\"column\":%u}",
            g_refs[i].resolved ? "true" : "false", g_refs[i].user_declared ? "true" : "false", g_refs[i].line,
            g_refs[i].column);
  }
  fputs("]}\n", stdout);
}

static long source_size(const char *path) {
  FILE *file = fopen(path, "rb");
  long size;
  if (!file) {
    return -1;
  }
  if (fseek(file, 0, SEEK_END) != 0) {
    fclose(file);
    return -1;
  }
  size = ftell(file);
  fclose(file);
  return size;
}

int main(int argc, char **argv) {
  long size;
  CXIndex index;
  CXTranslationUnit unit;
  int depth = 0;
  unsigned options = 0x01u | 0x200u; /* detailed preprocessing, keep going */
  if (argc < 2 || argv[1][0] == '\0' || argv[1][0] == '-') {
    fprintf(stderr, "usage: cpp-analyzer <source> [compiler-args...]\n");
    return 2;
  }
  size = source_size(argv[1]);
  if (size < 0) {
    emit_unsure("unreadable");
    return 0;
  }
  if (size > MAX_SOURCE) {
    emit_unsure("source-too-large");
    return 0;
  }
  index = clang_createIndex(0, 0);
  unit = clang_parseTranslationUnit(index, argv[1], (const char *const *)argv + 2, argc - 2, NULL, 0, options);
  if (!unit) {
    emit_unsure("parse-failed");
    clang_disposeIndex(index);
    return 0;
  }
  if (has_error(unit)) {
    emit_unsure("diagnostics");
    clang_disposeTranslationUnit(unit);
    clang_disposeIndex(index);
    return 0;
  }
  if (!find_main(unit, argv[1])) {
    emit_unsure("parse-failed");
    clang_disposeTranslationUnit(unit);
    clang_disposeIndex(index);
    return 0;
  }
  clang_getInclusions(unit, inclusion_visitor, NULL);
  resolve_headers();
  clang_visitChildren(clang_getTranslationUnitCursor(unit), visit, &depth);
  if (g_overflow) {
    emit_unsure("reference-bound");
  } else {
    emit_ok();
  }
  clang_disposeTranslationUnit(unit);
  clang_disposeIndex(index);
  return 0;
}
