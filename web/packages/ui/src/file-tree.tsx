import { ChevronRight, Folder, FolderOpen } from "lucide-react";
import { useMemo, type ReactNode } from "react";

export type FileTreeEntry = {
  id: string;
  name: string;
  path?: string;
};

type Directory<T> = {
  name: string;
  path: string;
  directories: Map<string, Directory<T>>;
  files: Array<{ file: T; name: string }>;
};

function directory<T>(name: string, path: string): Directory<T> {
  return { name, path, directories: new Map(), files: [] };
}

/** Paths are display metadata only; file actions always retain the original ID. */
function buildTree<T extends FileTreeEntry>(files: T[], rootPath: string) {
  const root = directory<T>(rootPath, rootPath);
  for (const file of files) {
    const relative = file.path?.startsWith(`${rootPath}/`)
      ? file.path.slice(rootPath.length + 1)
      : file.path && !file.path.startsWith("/")
        ? file.path
        : file.name;
    const parts = relative.split("/").filter((part) => part && part !== ".");
    // Unexpected paths stay visible at the root, without inventing parent folders.
    if (parts.includes("..")) {
      root.files.push({ file, name: file.name });
      continue;
    }
    const name = parts.pop() || file.name;
    let parent = root;
    for (const part of parts) {
      let child = parent.directories.get(part);
      if (!child) {
        child = directory<T>(part, `${parent.path}/${part}`);
        parent.directories.set(part, child);
      }
      parent = child;
    }
    parent.files.push({ file, name });
  }
  return root;
}

export function FileTree<T extends FileTreeEntry>({
  files,
  label,
  rootPath = "/workspace",
  locale,
  renderFile,
}: {
  files: T[];
  label: string;
  rootPath?: string;
  locale?: string;
  renderFile(file: T, name: string): ReactNode;
}) {
  const root = useMemo(() => buildTree(files, rootPath), [files, rootPath]);
  const collator = useMemo(
    () => new Intl.Collator(locale, { numeric: true }),
    [locale],
  );
  const renderDirectory = (node: Directory<T>, isRoot = false): ReactNode => (
    <li key={node.path}>
      <details
        className="argus-file-tree__directory"
        open={isRoot || undefined}
      >
        <summary className="argus-file-tree__folder" title={node.path}>
          <ChevronRight aria-hidden className="argus-file-tree__chevron" />
          <Folder aria-hidden className="argus-file-tree__closed-icon" />
          <FolderOpen aria-hidden className="argus-file-tree__open-icon" />
          <span className="argus-file-tree__name">{node.name}</span>
        </summary>
        <ul className="argus-file-tree__children">
          {[...node.directories.values()]
            .sort((a, b) => collator.compare(a.name, b.name))
            .map((child) => renderDirectory(child))}
          {[...node.files]
            .sort((a, b) => collator.compare(a.name, b.name))
            .map(({ file, name }) => (
              <li key={file.id}>{renderFile(file, name)}</li>
            ))}
        </ul>
      </details>
    </li>
  );
  return (
    <ul className="argus-file-tree" aria-label={label}>
      {renderDirectory(root, true)}
    </ul>
  );
}
