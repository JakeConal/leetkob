// leetkob-analyzer resolves ban-list names against the libclang AST.
//
// Input (stdin, bounded):
//   {"source":"<C++17 source>","bannedNames":["sort","std::sort"]}
//
// Output (stdout, bounded):
//   {"version":1,"results":[{"name":"std::sort","verdict":"violation|pass|unsure",
//                            "evidence":[{"line":3,"text":"  std::sort(v.begin(), v.end());"}]}]}
//
// The tool only reports a violation when libclang resolved a reference to
// a declaration with that name. Comments, string literals, an unused
// <algorithm> include and student-defined names never produce a
// violation. Anything unresolved becomes unsure. All original evidence is
// read from the submitted source, not from headers.
#include <clang-c/Index.h>

#include <algorithm>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <iostream>
#include <map>
#include <set>
#include <string>
#include <utility>
#include <vector>

#include "default_clang_args.inc"

namespace {

constexpr size_t kMaxSourceBytes = 65536;
constexpr size_t kMaxBannedNames = 64;
constexpr size_t kMaxNameBytes = 256;
constexpr size_t kMaxInputBytes = 256u << 10;
constexpr size_t kMaxOutputBytes = 2u << 20;
constexpr size_t kMaxEvidencePerName = 4;
constexpr size_t kMaxEvidenceTextBytes = 4096;
constexpr int kMaxJsonDepth = 32;

// ---------------------------------------------------------------------------
// Bounded JSON parsing (no external dependency)
// ---------------------------------------------------------------------------

std::string jsonEscape(const std::string& text) {
    static const char* hex = "0123456789abcdef";
    std::string out;
    out.reserve(text.size() + 2);
    for (unsigned char c : text) {
        switch (c) {
            case '"': out += "\\\""; break;
            case '\\': out += "\\\\"; break;
            case '\b': out += "\\b"; break;
            case '\f': out += "\\f"; break;
            case '\n': out += "\\n"; break;
            case '\r': out += "\\r"; break;
            case '\t': out += "\\t"; break;
            default:
                if (c < 0x20) {
                    out += "\\u00";
                    out += hex[(c >> 4) & 0x0f];
                    out += hex[c & 0x0f];
                } else {
                    out += static_cast<char>(c);
                }
        }
    }
    return out;
}

void appendUtf8(std::string& out, uint32_t codePoint) {
    if (codePoint <= 0x7f) {
        out += static_cast<char>(codePoint);
    } else if (codePoint <= 0x7ff) {
        out += static_cast<char>(0xc0 | (codePoint >> 6));
        out += static_cast<char>(0x80 | (codePoint & 0x3f));
    } else if (codePoint <= 0xffff) {
        out += static_cast<char>(0xe0 | (codePoint >> 12));
        out += static_cast<char>(0x80 | ((codePoint >> 6) & 0x3f));
        out += static_cast<char>(0x80 | (codePoint & 0x3f));
    } else {
        out += static_cast<char>(0xf0 | (codePoint >> 18));
        out += static_cast<char>(0x80 | ((codePoint >> 12) & 0x3f));
        out += static_cast<char>(0x80 | ((codePoint >> 6) & 0x3f));
        out += static_cast<char>(0x80 | (codePoint & 0x3f));
    }
}

class JsonParser {
public:
    explicit JsonParser(const std::string& text) : text_(text) {}

    bool parseRequest(std::string& source, std::vector<std::string>& names, std::string& error) {
        skipWhitespace();
        if (!consume('{')) {
            error = "request must be a JSON object";
            return false;
        }
        bool haveSource = false;
        bool haveNames = false;
        skipWhitespace();
        if (peek() == '}') {
            ++pos_;
        } else {
            while (true) {
                std::string key;
                if (!parseString(key)) {
                    error = "invalid object key";
                    return false;
                }
                skipWhitespace();
                if (!consume(':')) {
                    error = "expected ':'";
                    return false;
                }
                skipWhitespace();
                if (key == "source") {
                    if (haveSource || !parseString(source)) {
                        error = "invalid source field";
                        return false;
                    }
                    haveSource = true;
                } else if (key == "bannedNames") {
                    if (haveNames || !parseStringArray(names)) {
                        error = "invalid bannedNames field";
                        return false;
                    }
                    haveNames = true;
                } else if (!skipValue(0)) {
                    error = "invalid unknown field";
                    return false;
                }
                skipWhitespace();
                if (consume(',')) {
                    skipWhitespace();
                    continue;
                }
                if (consume('}')) {
                    break;
                }
                error = "expected ',' or '}'";
                return false;
            }
        }
        skipWhitespace();
        if (pos_ != text_.size()) {
            error = "trailing data after request object";
            return false;
        }
        if (!haveSource || !haveNames) {
            error = "source and bannedNames are required";
            return false;
        }
        return true;
    }

private:
    const std::string& text_;
    size_t pos_ = 0;

    char peek() const { return pos_ < text_.size() ? text_[pos_] : '\0'; }

    void skipWhitespace() {
        while (pos_ < text_.size()) {
            const char c = text_[pos_];
            if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
                ++pos_;
            } else {
                break;
            }
        }
    }

    bool consume(char expected) {
        if (peek() != expected) {
            return false;
        }
        ++pos_;
        return true;
    }

    bool parseString(std::string& out) {
        out.clear();
        if (!consume('"')) {
            return false;
        }
        while (pos_ < text_.size()) {
            unsigned char c = static_cast<unsigned char>(text_[pos_++]);
            if (c == '"') {
                return true;
            }
            if (c < 0x20) {
                return false;
            }
            if (c != '\\') {
                out += static_cast<char>(c);
                continue;
            }
            if (pos_ >= text_.size()) {
                return false;
            }
            const char escape = text_[pos_++];
            switch (escape) {
                case '"': out += '"'; break;
                case '\\': out += '\\'; break;
                case '/': out += '/'; break;
                case 'b': out += '\b'; break;
                case 'f': out += '\f'; break;
                case 'n': out += '\n'; break;
                case 'r': out += '\r'; break;
                case 't': out += '\t'; break;
                case 'u': {
                    uint32_t codePoint = 0;
                    if (!parseHex4(codePoint)) {
                        return false;
                    }
                    if (codePoint >= 0xd800 && codePoint <= 0xdbff) {
                        if (pos_ + 1 >= text_.size() || text_[pos_] != '\\' || text_[pos_ + 1] != 'u') {
                            return false;
                        }
                        pos_ += 2;
                        uint32_t low = 0;
                        if (!parseHex4(low) || low < 0xdc00 || low > 0xdfff) {
                            return false;
                        }
                        codePoint = 0x10000 + ((codePoint - 0xd800) << 10) + (low - 0xdc00);
                    } else if (codePoint >= 0xdc00 && codePoint <= 0xdfff) {
                        return false;
                    }
                    appendUtf8(out, codePoint);
                    break;
                }
                default:
                    return false;
            }
        }
        return false;
    }

    bool parseHex4(uint32_t& value) {
        if (pos_ + 4 > text_.size()) {
            return false;
        }
        value = 0;
        for (int i = 0; i < 4; ++i) {
            const char c = text_[pos_++];
            value <<= 4;
            if (c >= '0' && c <= '9') {
                value += static_cast<uint32_t>(c - '0');
            } else if (c >= 'a' && c <= 'f') {
                value += static_cast<uint32_t>(c - 'a' + 10);
            } else if (c >= 'A' && c <= 'F') {
                value += static_cast<uint32_t>(c - 'A' + 10);
            } else {
                return false;
            }
        }
        return true;
    }

    bool parseStringArray(std::vector<std::string>& out) {
        out.clear();
        if (!consume('[')) {
            return false;
        }
        skipWhitespace();
        if (consume(']')) {
            return true;
        }
        while (true) {
            std::string value;
            if (!parseString(value)) {
                return false;
            }
            out.push_back(std::move(value));
            if (out.size() > kMaxBannedNames) {
                return false;
            }
            skipWhitespace();
            if (consume(',')) {
                skipWhitespace();
                continue;
            }
            if (consume(']')) {
                return true;
            }
            return false;
        }
    }

    bool skipValue(int depth) {
        if (depth > kMaxJsonDepth) {
            return false;
        }
        skipWhitespace();
        const char c = peek();
        if (c == '"') {
            std::string ignored;
            return parseString(ignored);
        }
        if (c == '{') {
            ++pos_;
            skipWhitespace();
            if (consume('}')) {
                return true;
            }
            while (true) {
                std::string key;
                if (!parseString(key)) {
                    return false;
                }
                skipWhitespace();
                if (!consume(':') || !skipValue(depth + 1)) {
                    return false;
                }
                skipWhitespace();
                if (consume(',')) {
                    skipWhitespace();
                    continue;
                }
                return consume('}');
            }
        }
        if (c == '[') {
            ++pos_;
            skipWhitespace();
            if (consume(']')) {
                return true;
            }
            while (true) {
                if (!skipValue(depth + 1)) {
                    return false;
                }
                skipWhitespace();
                if (consume(',')) {
                    skipWhitespace();
                    continue;
                }
                return consume(']');
            }
        }
        for (const char* literal : {"true", "false", "null"}) {
            const size_t length = std::strlen(literal);
            if (text_.compare(pos_, length, literal) == 0) {
                pos_ += length;
                return true;
            }
        }
        const size_t start = pos_;
        if (peek() == '-') {
            ++pos_;
        }
        bool digits = false;
        while (pos_ < text_.size() && text_[pos_] >= '0' && text_[pos_] <= '9') {
            ++pos_;
            digits = true;
        }
        if (pos_ < text_.size() && text_[pos_] == '.') {
            ++pos_;
            while (pos_ < text_.size() && text_[pos_] >= '0' && text_[pos_] <= '9') {
                ++pos_;
            }
        }
        if (pos_ < text_.size() && (text_[pos_] == 'e' || text_[pos_] == 'E')) {
            ++pos_;
            if (pos_ < text_.size() && (text_[pos_] == '+' || text_[pos_] == '-')) {
                ++pos_;
            }
            while (pos_ < text_.size() && text_[pos_] >= '0' && text_[pos_] <= '9') {
                ++pos_;
            }
        }
        return digits && pos_ > start;
    }
};

// ---------------------------------------------------------------------------
// Source and name helpers
// ---------------------------------------------------------------------------

std::vector<std::string> splitLines(const std::string& source) {
    std::vector<std::string> lines;
    size_t start = 0;
    while (true) {
        const size_t newline = source.find('\n', start);
        std::string line = newline == std::string::npos
                               ? source.substr(start)
                               : source.substr(start, newline - start);
        if (!line.empty() && line.back() == '\r') {
            line.pop_back();
        }
        lines.push_back(std::move(line));
        if (newline == std::string::npos) {
            break;
        }
        start = newline + 1;
    }
    return lines;
}

std::string truncateUtf8(const std::string& text, size_t limit) {
    if (text.size() <= limit) {
        return text;
    }
    size_t cut = limit;
    while (cut > 0 && (static_cast<unsigned char>(text[cut]) & 0xc0) == 0x80) {
        --cut;
    }
    return text.substr(0, cut);
}

std::string trim(const std::string& text) {
    size_t begin = 0;
    size_t end = text.size();
    while (begin < end && (text[begin] == ' ' || text[begin] == '\t' || text[begin] == '\n' || text[begin] == '\r')) {
        ++begin;
    }
    while (end > begin && (text[end - 1] == ' ' || text[end - 1] == '\t' || text[end - 1] == '\n' || text[end - 1] == '\r')) {
        --end;
    }
    return text.substr(begin, end - begin);
}

std::string normalizeName(const std::string& raw) {
    std::string name;
    name.reserve(raw.size());
    for (unsigned char c : raw) {
        if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
            continue;
        }
        name += static_cast<char>(c);
    }
    while (name.rfind("::", 0) == 0) {
        name.erase(0, 2);
    }
    if (name.size() >= 2 && name.compare(name.size() - 2, 2, "()") == 0) {
        name.resize(name.size() - 2);
    }
    return name;
}

std::string lastComponent(const std::string& name) {
    const size_t pos = name.rfind("::");
    return pos == std::string::npos ? name : name.substr(pos + 2);
}

bool isQualified(const std::string& name) {
    return name.find("::") != std::string::npos;
}

std::string cxString(CXString value) {
    const char* text = clang_getCString(value);
    std::string out = text == nullptr ? std::string() : std::string(text);
    clang_disposeString(value);
    return out;
}

std::string cursorSpelling(CXCursor cursor) {
    return cxString(clang_getCursorSpelling(cursor));
}

bool isRecordOrNamespace(CXCursorKind kind) {
    switch (kind) {
        case CXCursor_Namespace:
        case CXCursor_StructDecl:
        case CXCursor_ClassDecl:
        case CXCursor_UnionDecl:
        case CXCursor_EnumDecl:
        case CXCursor_ClassTemplate:
        case CXCursor_ClassTemplatePartialSpecialization:
            return true;
        default:
            return false;
    }
}

std::string qualifiedName(CXCursor cursor) {
    std::string name = cursorSpelling(cursor);
    CXCursor parent = clang_getCursorSemanticParent(cursor);
    for (int depth = 0; depth < 32; ++depth) {
        if (clang_Cursor_isNull(parent) || !isRecordOrNamespace(clang_getCursorKind(parent))) {
            break;
        }
        const std::string parentName = cursorSpelling(parent);
        if (!parentName.empty()) {
            name = parentName + "::" + name;
        }
        parent = clang_getCursorSemanticParent(parent);
    }
    return name;
}

bool fileEquals(CXFile file, CXFile other) {
    if (file == nullptr || other == nullptr) {
        return false;
    }
    if (file == other) {
        return true;
    }
    const std::string a = cxString(clang_getFileName(file));
    const std::string b = cxString(clang_getFileName(other));
    return !a.empty() && a == b;
}

bool fileIsMain(CXFile file, CXFile mainFile) {
    if (file == nullptr) {
        return false;
    }
    if (mainFile != nullptr && fileEquals(file, mainFile)) {
        return true;
    }
    // The unsaved translation unit is named "main.cpp"; some libclang
    // builds report it as an absolute path or return no CXFile from
    // clang_getFile for the unsaved buffer. The name fallback keeps
    // evidence tied to the submitted source either way.
    const std::string name = cxString(clang_getFileName(file));
    return name == "main.cpp" ||
           (name.size() > 9 && name.compare(name.size() - 9, 9, "/main.cpp") == 0);
}

bool declarationInMainFile(CXCursor referenced, CXFile mainFile) {
    CXCursor declaration = clang_getCursorDefinition(referenced);
    if (clang_Cursor_isNull(declaration)) {
        declaration = referenced;
    }
    CXFile file = nullptr;
    unsigned line = 0;
    unsigned column = 0;
    unsigned offset = 0;
    clang_getSpellingLocation(clang_getCursorLocation(declaration), &file, &line, &column, &offset);
    return fileIsMain(file, mainFile);
}

bool referenceInMainFile(CXCursor reference, CXFile mainFile, unsigned& line) {
    CXSourceLocation location = clang_getCursorLocation(reference);
    CXFile file = nullptr;
    unsigned column = 0;
    unsigned offset = 0;
    clang_getExpansionLocation(location, &file, &line, &column, &offset);
    if (file == nullptr || line == 0) {
        clang_getSpellingLocation(location, &file, &line, &column, &offset);
    }
    return line > 0 && fileIsMain(file, mainFile);
}

std::string sourceLine(const std::vector<std::string>& lines, unsigned line) {
    if (line == 0 || line > lines.size()) {
        return std::string();
    }
    return lines[line - 1];
}

// ---------------------------------------------------------------------------
// AST analysis
// ---------------------------------------------------------------------------

struct Ban {
    std::string wireName;
    std::string normalized;
};

struct Evidence {
    unsigned line = 0;
    std::string text;
};

struct Analysis {
    std::vector<Evidence> evidence;
    bool matched = false;
    bool unsure = false;
};

struct VisitContext {
    CXFile mainFile = nullptr;
    const std::vector<std::string>* lines = nullptr;
    const std::vector<Ban>* bans = nullptr;
    std::vector<Analysis>* results = nullptr;
};

void noteMatchingBans(VisitContext& context, const std::string& spelling, bool unsure) {
    if (spelling.empty()) {
        return;
    }
    const std::string last = lastComponent(spelling);
    for (size_t i = 0; i < context.bans->size(); ++i) {
        const Ban& ban = (*context.bans)[i];
        if (lastComponent(ban.normalized) != last) {
            continue;
        }
        if (unsure) {
            (*context.results)[i].unsure = true;
        }
    }
}

void recordEvidence(VisitContext& context, size_t banIndex, CXCursor reference) {
    Analysis& analysis = (*context.results)[banIndex];
    unsigned line = 0;
    if (!referenceInMainFile(reference, context.mainFile, line)) {
        analysis.unsure = true;
        return;
    }
    const std::string text = sourceLine(*context.lines, line);
    if (text.empty()) {
        analysis.unsure = true;
        return;
    }
    Evidence evidence;
    evidence.line = line;
    evidence.text = truncateUtf8(text, kMaxEvidenceTextBytes);
    for (const Evidence& existing : analysis.evidence) {
        if (existing.line == evidence.line && existing.text == evidence.text) {
            analysis.matched = true;
            return;
        }
    }
    analysis.evidence.push_back(std::move(evidence));
    analysis.matched = true;
}

void processReferenced(VisitContext& context, CXCursor reference, CXCursor referenced) {
    if (clang_Cursor_isNull(referenced)) {
        return;
    }
    // A name defined in the submitted source is student-defined, even if
    // it happens to be spelled std::sort in a (non-portable) namespace.
    if (declarationInMainFile(referenced, context.mainFile)) {
        return;
    }
    std::string qualified = qualifiedName(referenced);
    if (qualified.empty()) {
        qualified = cursorSpelling(referenced);
    }
    if (qualified.empty()) {
        noteMatchingBans(context, cursorSpelling(reference), true);
        return;
    }
    const std::string last = lastComponent(qualified);
    for (size_t i = 0; i < context.bans->size(); ++i) {
        const Ban& ban = (*context.bans)[i];
        const bool matches = isQualified(ban.normalized)
                                 ? qualified == ban.normalized
                                 : last == ban.normalized;
        if (matches) {
            recordEvidence(context, i, reference);
        }
    }
}

void processReferenceCursor(VisitContext& context, CXCursor cursor) {
    // Only references written in the submitted source matter. The AST also
    // contains library-header bodies and declarations; an internal call in
    // <algorithm> is not the student's code and must not turn a harmless
    // include into unsure or a violation.
    unsigned referenceLine = 0;
    if (!referenceInMainFile(cursor, context.mainFile, referenceLine)) {
        return;
    }
    (void)referenceLine;
    const CXCursor referenced = clang_getCursorReferenced(cursor);
    if (!clang_Cursor_isNull(referenced)) {
        processReferenced(context, cursor, referenced);
        return;
    }
    const unsigned overloads = clang_getNumOverloadedDecls(cursor);
    if (overloads > 0) {
        for (unsigned i = 0; i < overloads; ++i) {
            processReferenced(context, cursor, clang_getOverloadedDecl(cursor, i));
        }
        return;
    }
    // An unresolved reference is ambiguous. If its spelling could name a
    // banned declaration, the safe answer is unsure, not pass.
    noteMatchingBans(context, cursorSpelling(cursor), true);
}

CXChildVisitResult visitCursor(CXCursor cursor, CXCursor /*parent*/, CXClientData clientData) {
    auto* context = static_cast<VisitContext*>(clientData);
    switch (clang_getCursorKind(cursor)) {
        case CXCursor_DeclRefExpr:
        case CXCursor_MemberRefExpr:
        case CXCursor_TypeRef:
        case CXCursor_TemplateRef:
        case CXCursor_OverloadedDeclRef:
        case CXCursor_VariableRef:
            processReferenceCursor(*context, cursor);
            break;
        default:
            break;
    }
    return CXChildVisit_Recurse;
}

size_t countErrorDiagnostics(CXTranslationUnit translationUnit) {
    size_t errors = 0;
    const unsigned count = clang_getNumDiagnostics(translationUnit);
    for (unsigned i = 0; i < count; ++i) {
        CXDiagnostic diagnostic = clang_getDiagnostic(translationUnit, i);
        if (clang_getDiagnosticSeverity(diagnostic) >= CXDiagnostic_Error) {
            ++errors;
        }
        clang_disposeDiagnostic(diagnostic);
    }
    return errors;
}

class TranslationUnit {
public:
    TranslationUnit() = default;
    TranslationUnit(const TranslationUnit&) = delete;
    TranslationUnit& operator=(const TranslationUnit&) = delete;
    ~TranslationUnit() {
        if (translationUnit_ != nullptr) {
            clang_disposeTranslationUnit(translationUnit_);
        }
        if (index_ != nullptr) {
            clang_disposeIndex(index_);
        }
    }

    bool parse(const std::string& source) {
        index_ = clang_createIndex(0, 0);
        if (index_ == nullptr) {
            return false;
        }
        std::vector<const char*> arguments;
        arguments.push_back("-std=c++17");
        arguments.push_back("-w");
        arguments.push_back("-ferror-limit=0");
        for (const char* argument : kDefaultClangArgs) {
            arguments.push_back(argument);
        }
        CXUnsavedFile unsaved{};
        unsaved.Filename = "main.cpp";
        unsaved.Contents = source.c_str();
        unsaved.Length = static_cast<unsigned long>(source.size());
        const CXErrorCode error = clang_parseTranslationUnit2(
            index_, "main.cpp", arguments.data(), static_cast<int>(arguments.size()),
            &unsaved, 1, CXTranslationUnit_None, &translationUnit_);
        return error == CXError_Success && translationUnit_ != nullptr;
    }

    CXTranslationUnit get() const { return translationUnit_; }

private:
    CXIndex index_ = nullptr;
    CXTranslationUnit translationUnit_ = nullptr;
};

struct Result {
    std::string name;
    std::string verdict;
    std::vector<Evidence> evidence;
};

std::string render(const std::vector<Result>& results) {
    std::string out;
    out.reserve(256 + results.size() * 128);
    out += "{\"version\":1,\"results\":[";
    bool first = true;
    for (const Result& result : results) {
        if (!first) {
            out += ',';
        }
        first = false;
        out += "{\"name\":\"";
        out += jsonEscape(result.name);
        out += "\",\"verdict\":\"";
        out += result.verdict;
        out += "\",\"evidence\":[";
        bool firstEvidence = true;
        for (const Evidence& evidence : result.evidence) {
            if (!firstEvidence) {
                out += ',';
            }
            firstEvidence = false;
            out += "{\"line\":";
            out += std::to_string(evidence.line);
            out += ",\"text\":\"";
            out += jsonEscape(evidence.text);
            out += "\"}";
        }
        out += "]}";
    }
    out += "]}\n";
    return out;
}

std::vector<Result> unsureResults(const std::vector<Ban>& bans) {
    std::vector<Result> results;
    results.reserve(bans.size());
    for (const Ban& ban : bans) {
        results.push_back(Result{ban.wireName, "unsure", {}});
    }
    return results;
}

std::vector<Result> analyze(const std::string& source, const std::vector<Ban>& bans) {
    if (bans.empty()) {
        return {};
    }
    const std::vector<std::string> lines = splitLines(source);
    TranslationUnit unit;
    if (!unit.parse(source)) {
        return unsureResults(bans);
    }

    std::vector<Analysis> analyses(bans.size());
    VisitContext context;
    context.mainFile = clang_getFile(unit.get(), "main.cpp");
    context.lines = &lines;
    context.bans = &bans;
    context.results = &analyses;
    clang_visitChildren(clang_getTranslationUnitCursor(unit.get()), visitCursor, &context);
    const size_t errors = countErrorDiagnostics(unit.get());

    std::vector<Result> results;
    results.reserve(bans.size());
    for (size_t i = 0; i < bans.size(); ++i) {
        Analysis& analysis = analyses[i];
        std::sort(analysis.evidence.begin(), analysis.evidence.end(),
                  [](const Evidence& a, const Evidence& b) { return a.line < b.line; });
        if (analysis.evidence.size() > kMaxEvidencePerName) {
            analysis.evidence.resize(kMaxEvidencePerName);
        }
        Result result;
        result.name = bans[i].wireName;
        if (analysis.matched && !analysis.evidence.empty()) {
            result.verdict = "violation";
            result.evidence = analysis.evidence;
        } else if (analysis.matched || analysis.unsure || errors > 0) {
            result.verdict = "unsure";
        } else {
            result.verdict = "pass";
        }
        results.push_back(std::move(result));
    }
    return results;
}

}  // namespace

int main() {
    std::string input;
    char buffer[8192];
    bool tooLarge = false;
    while (std::cin) {
        std::cin.read(buffer, sizeof(buffer));
        const std::streamsize read = std::cin.gcount();
        if (read <= 0) {
            break;
        }
        if (input.size() + static_cast<size_t>(read) > kMaxInputBytes) {
            tooLarge = true;
            break;
        }
        input.append(buffer, static_cast<size_t>(read));
    }
    if (tooLarge) {
        std::fprintf(stderr, "analyzer: request exceeds input bound\n");
        return 2;
    }

    JsonParser parser(input);
    std::string source;
    std::vector<std::string> rawNames;
    std::string error;
    if (!parser.parseRequest(source, rawNames, error)) {
        std::fprintf(stderr, "analyzer: invalid request: %s\n", error.c_str());
        return 2;
    }
    if (source.size() > kMaxSourceBytes) {
        std::fprintf(stderr, "analyzer: source exceeds %zu bytes\n", kMaxSourceBytes);
        return 2;
    }

    // Keep first-seen wire spellings. Normalization is only for matching;
    // the Go boundary compares result names with the names it sent.
    std::vector<Ban> bans;
    std::set<std::string> seen;
    for (const std::string& raw : rawNames) {
        const std::string trimmed = trim(raw);
        if (trimmed.empty() || trimmed.size() > kMaxNameBytes) {
            std::fprintf(stderr, "analyzer: invalid banned name\n");
            return 2;
        }
        if (!seen.insert(trimmed).second) {
            continue;
        }
        bans.push_back(Ban{trimmed, normalizeName(trimmed)});
    }

    const std::vector<Result> results = analyze(source, bans);
    const std::string output = render(results);
    if (output.size() > kMaxOutputBytes) {
        // Bounded output is more important than a detailed answer. An
        // all-unsure reply can never cause a wrongful deduction.
        const std::string fallback = render(unsureResults(bans));
        std::cout.write(fallback.data(), static_cast<std::streamsize>(fallback.size()));
        return 0;
    }
    std::cout.write(output.data(), static_cast<std::streamsize>(output.size()));
    return 0;
}
