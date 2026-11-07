# Plan skill — organize sequential page_*.txt files into structural subdirectories

This repository sometimes accumulates large numbers of sequential page
files (e.g. `page_0001.txt`, `page_0002.txt`, …) extracted from a
scanned technical manual.  This skill describes how to inspect those
pages, infer the document's logical structure, move them into
subdirectories (`front_matter`, `table_of_contents`, `chapter_N`,
`appendix_N`, `back_matter`), and write a master `toc.md` that links
into the new layout.

Use this skill whenever the task says "move the page files into
subdirectories" or "write a master table of contents" for a directory
of `page_*.txt` files.

## When to use

- User says: *organize the pages into subdirectories*, *move the pages
  into chapter folders*, *write a master table of contents*, or
  otherwise asks to restructure a flat `page_NNN.txt` collection.
- User has already created `chapter_NN/page_NNN.txt` subdirectories
  and wants the remaining root-level pages classified.

## Inputs the user has already established

Before organizing, confirm:

- The source directory contains only sequential `page_*.txt` files
  (and possibly an `imgs/` folder that should stay in place).
- Page filenames sort lexicographically in the same order they appear
  in the physical document.  Padding varies by document: some use
  `page_001.txt`, others `page_0001.txt`, others `page_1.txt`.
- Pages are OCR output and therefore contain form-feed characters
  (`\f`), repeated section headers, stray blank lines, and occasional
  empty files.

## Procedure

1. **Survey the boundaries.**  Read the first and last few pages to
   identify title-page text, copyright blocks, publisher info, and
   back-matter markers (index, bibliography, revision page, blank
   runs, "USNRC HRTD", etc.).

2. **Find structural markers.**  Grep for strings that announce a new
   region.  Patterns vary by publisher:

   ```bash
   # Westinghouse manuals
   grep -l "Westinghouse Technology Systems Manual$" page_*.txt
   grep -l "TABLE OF CONTENTS" page_*.txt

   # IAEA / other manuals
   grep -l "^[0-9]\+\.[0-9]* \|^Appendix\|^CONTENTS\|TABLE OF" page_*.txt

   # Common front/back matter
   grep -l "ABSTRACT\|PREFACE\|ACKNOWLEDGMENT\|FOREWORD\|EDITORIAL" page_*.txt
   grep -l "BIBLIOGRAPHY\|INDEX\|APPENDIX" page_*.txt
   ```

   A page whose first non-blank line is a known publisher header
   (e.g. `Westinghouse Technology Systems Manual`) is almost always a
   chapter-title or section-title page.  The second line usually reads
   `Chapter N`, `Section M.K`, or similar.  For manuals without a
   repeating publisher header, look for the highest-numbered section
   on a page (e.g. `5. MODEL DESCRIPTION`) as a boundary signal.

3. **Build the page-to-section map.**  For every marker page, record
   the page number, the directory it currently lives in (root or an
   existing subdirectory), and the header text.  Sort the list
   numerically; each entry marks the *start* of a new section, and the
   previous entry's start page is the end of the preceding section
   minus one.

4. **Account for gaps and duplicates.**  Pages that belong to the
   physical document but are missing from the filesystem are normal.
   Pages whose headers repeat the previous section (e.g. a "TABLE OF
   CONTENTS" page that appears at the start of each chapter) should be
   assigned to the section that follows the header, not to the
   preceding one.  Blank or form-feed-only pages belong at the end of
   the section they interrupt, preserving page numbering and spacing
   for downstream cross-referencing.

5. **Create target directories.**  Use short, snake_case names:

   ```
   front_matter/        pages before the main TOC
   table_of_contents/   the primary TOC (may span multiple pages)
   chapter_N/           each top-level numbered chapter or section
   appendix_N/          appendices that are not part of the chapter sequence
   back_matter/         pages after the last chapter (blank pages,
                        index, bibliography, revision blocks)
   ```

   If the manual was published with gaps in its chapter numbering
   (e.g. no Chapter 15), do not create empty directories for missing
   chapters.  When the manual uses "Section" numbering rather than
   "Chapter" numbering, prefer `chapter_N` to keep directory names
   consistent across reorganized collections unless the user explicitly
   asks for `section_N`.

6. **Move files in bulk.**  A single shell loop is sufficient.  Use
   `printf "page_%03d.txt"` (or `%04d`) matching the source directory's
   zero-padding, so that shell globbing does not mis-handle numbers.
   Verify with a per-directory count:

   ```bash
   for d in front_matter table_of_contents chapter_* appendix_* back_matter; do
     echo "$d: $(ls $d/*.txt 2>/dev/null | wc -l) files"
   done
   ```

7. **Write the master `toc.md`.**  Place it at the root of the page
   directory (next to the subdirectories).  Use the format:

   ```markdown
   # <Document Title> - Master Table of Contents

   ## <Region Name>

   - [<Short label>](<dir>/page_NNNN.txt) · NNNN–NNNN
   ```

   Rules for the TOC:

   - Group by chapter/section header pages found in step 2.
   - Show a *page range* (start–end) for each top-level section so the
     reader knows how many pages to expect.
   - Link to the section-header page itself, not to the first body
     page; that page carries the official title.
   - Keep `front_matter`, `table_of_contents`, and `back_matter` as
     top-level sections, not subsections of Chapter 1.
   - When a section title spans two lines (form-feed split), concatenate
     them with a space before writing the link text.
   - The document's own table of contents (if it lists subsections)
     may be transcribed into the master TOC as nested items under the
     relevant chapter, linking to the starting page of each subsection.

8. **Verify consistency.**  The sum of files across all subdirectories
   must equal the original file count.  The first file in
   `front_matter` should be the document's title page; the last file
   in `chapter_N/` or `appendix_N/` should be the page immediately
   before the next top-level section's title page.

## Outputs

- A set of subdirectories containing all `page_*.txt` files.
- A new `toc.md` at the root of the page tree with markdown links into
  every section.

## Conventions

- Do not rename or renumber page files; keep the original filename so
  that external references (issue numbers, commit hashes, image
  cross-references) remain valid.
- Chapter directories are named `chapter_N` with a leading zero only
  when the original document's printed numbering uses one (most manuals
  use `chapter_1`, `chapter_10`, not `chapter_01`).
- Use `appendix_N` for material that is supplementary, unnumbered, or
  appears after the main chapter sequence.
- Images in the `imgs/` folder stay in place regardless of page
  reorganization; their filenames usually map by page number
  (`017.png` → page 00017).

## Anti-patterns to avoid

- Deleting blank or form-feed-only pages.  They preserve page numbering
  and spacing.
- Mixing content pages with the TOC index page.  The TOC is a single
  logical unit; all other `TABLE OF CONTENTS` repeats belong to the
  section they introduce.
- Guessing chapter boundaries from page counts.  Always use header text
  as the source of truth.
- Writing a TOC entry for every page.  The TOC should list *sections*,
  not 1,100 individual files.

## Reading an existing organization back

When asked to *use* the organized pages (e.g. "what does chapter 5
say about X?"):

1. Read `toc.md` to find the relevant section and page range.
2. Open the section's header page first to get the formal title and
   figure/table list.
3. Grep the section's directory for the term of interest; page files
   are small enough to read in full.