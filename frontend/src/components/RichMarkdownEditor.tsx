import {
  BlockTypeSelect,
  BoldItalicUnderlineToggles,
  CodeToggle,
  CreateLink,
  InsertImage,
  InsertTable,
  InsertThematicBreak,
  ListsToggle,
  MDXEditor,
  type MDXEditorMethods,
  UndoRedo,
  codeBlockPlugin,
  codeMirrorPlugin,
  headingsPlugin,
  imagePlugin,
  linkDialogPlugin,
  linkPlugin,
  listsPlugin,
  markdownShortcutPlugin,
  quotePlugin,
  tablePlugin,
  thematicBreakPlugin,
  toolbarPlugin,
} from "@mdxeditor/editor";
import "@mdxeditor/editor/style.css";
import { useEffect, useMemo, useRef, useState } from "react";

const MAX_IMAGE_BYTES = 4 * 1024 * 1024;

function readImageAsDataURL(file: File): Promise<string> {
  if (!file.type.startsWith("image/")) {
    return Promise.reject(new Error("只能插入图片文件"));
  }
  if (file.size > MAX_IMAGE_BYTES) {
    return Promise.reject(new Error("单张图片不能超过 4 MB"));
  }
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(new Error("读取图片失败"));
    reader.readAsDataURL(file);
  });
}

export interface RichMarkdownEditorProps {
  value: string;
  disabled?: boolean;
  placeholder?: string;
  compact?: boolean;
  onCommit(value: string): void;
}

export function RichMarkdownEditor({
  value,
  disabled = false,
  placeholder = "输入正文，支持 Markdown，也可以粘贴或拖入图片。",
  compact = false,
  onCommit,
}: RichMarkdownEditorProps) {
  const editorRef = useRef<MDXEditorMethods>(null);
  const [draft, setDraft] = useState(value);
  const [sourceMode, setSourceMode] = useState(false);
  const latestDraft = useRef(value);

  useEffect(() => {
    if (value === latestDraft.current) return;
    latestDraft.current = value;
    setDraft(value);
    editorRef.current?.setMarkdown(value);
  }, [value]);

  const plugins = useMemo(
    () => [
      headingsPlugin(),
      listsPlugin(),
      quotePlugin(),
      linkPlugin(),
      linkDialogPlugin(),
      tablePlugin(),
      thematicBreakPlugin(),
      imagePlugin({ imageUploadHandler: readImageAsDataURL }),
      codeBlockPlugin(),
      codeMirrorPlugin({
        codeBlockLanguages: {
          "": "纯文本",
          markdown: "Markdown",
        },
      }),
      markdownShortcutPlugin(),
      toolbarPlugin({
        toolbarClassName: "rich-editor-toolbar",
        toolbarContents: () => (
          <>
            <UndoRedo />
            <BlockTypeSelect />
            <BoldItalicUnderlineToggles />
            <CodeToggle />
            <ListsToggle />
            <CreateLink />
            <InsertImage />
            <InsertTable />
            <InsertThematicBreak />
          </>
        ),
      }),
    ],
    [],
  );

  const commit = () => {
    const normalized = draft.trim();
    if (normalized !== value) onCommit(normalized);
  };

  return (
    <div
      className={`rich-editor ${compact ? "compact" : ""} ${
        disabled ? "disabled" : ""
      }`}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) commit();
      }}
    >
      {!disabled && (
        <div className="editor-mode-tabs">
          <button
            type="button"
            className={!sourceMode ? "active" : ""}
            onClick={() => setSourceMode(false)}
          >
            富文本
          </button>
          <button
            type="button"
            className={sourceMode ? "active" : ""}
            onClick={() => setSourceMode(true)}
          >
            Markdown
          </button>
        </div>
      )}
      {sourceMode && !disabled ? (
        <textarea
          className="markdown-source-editor"
          value={draft}
          placeholder={placeholder}
          spellCheck={false}
          onChange={(event) => {
            latestDraft.current = event.target.value;
            setDraft(event.target.value);
          }}
        />
      ) : (
        <MDXEditor
          key={`${sourceMode}-${disabled}`}
          ref={editorRef}
          markdown={draft}
          readOnly={disabled}
          placeholder={placeholder}
          contentEditableClassName="rich-editor-content"
          onChange={(markdown) => {
            latestDraft.current = markdown;
            setDraft(markdown);
          }}
          plugins={plugins}
        />
      )}
    </div>
  );
}
