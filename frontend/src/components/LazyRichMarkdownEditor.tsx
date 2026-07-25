import { lazy, Suspense } from "react";
import type { RichMarkdownEditorProps } from "./RichMarkdownEditor";

const RichMarkdownEditor = lazy(() =>
  import("./RichMarkdownEditor").then((module) => ({
    default: module.RichMarkdownEditor,
  })),
);

export function LazyRichMarkdownEditor(props: RichMarkdownEditorProps) {
  return (
    <Suspense
      fallback={
        <div className="rich-editor rich-editor-loading">
          正在加载 Markdown 编辑器…
        </div>
      }
    >
      <RichMarkdownEditor {...props} />
    </Suspense>
  );
}
