#!/usr/bin/env python3
"""
preprocess.py — Script de preprocesamiento para PDFs con tablas complejas.

Uso:
    python scripts/preprocess.py data/pdfs/report.pdf
    python scripts/preprocess.py data/pdfs/report.pdf --out data/processed/report.json

Qué hace:
    1. Extrae texto con PyMuPDF (mismo motor C que go-fitz, misma calidad)
    2. Detecta tablas con page.find_tables() — esto es lo que go-fitz NO puede hacer
    3. Clasifica bloques por font-size (headings) igual que el extractor Go
    4. Construye SectionPath acumulado para cada elemento
    5. Serializa a JSON con el mismo schema que domain.Document en Go

Por qué existe:
    go-fitz extrae texto igual de bien que PyMuPDF porque es el mismo motor MuPDF.
    La única ventaja real de Python acá es find_tables(), que usa análisis de
    coordenadas para detectar celdas, filas y columnas en tablas sin bordes visibles.
    Para PDFs donde las tablas son el contenido crítico (reportes, especificaciones),
    este script agrega calidad real. Para el resto, Go hace todo solo.

Output:
    JSON con el mismo schema que domain.Document en Go.
    Go lee este archivo antes de intentar extraer con go-fitz.
    Si el archivo existe en data/processed/, Go lo usa directamente.

Dependencias:
    pip install pymupdf
"""

import argparse
import json
import hashlib
import os
import statistics
import sys
import time
import uuid
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Optional

try:
    import fitz  # PyMuPDF
except ImportError:
    print("Error: PyMuPDF no está instalado. Ejecutá: pip install pymupdf", file=sys.stderr)
    sys.exit(1)


# ─── Tipos (espejo exacto de domain en Go) ────────────────────────────────────

@dataclass
class Element:
    type: str           # "heading" | "paragraph" | "table" | "list_item"
    text: str           # texto plano (tablas: Markdown serializado)
    page: int           # 1-based
    level: int = 0      # solo headings: 1-6
    cells: list = field(default_factory=list)         # solo tablas: [[row][col]]
    section_path: list = field(default_factory=list)  # ["Cap 3", "3.1 Instalación"]


@dataclass
class DocumentMetadata:
    source: str
    doc_type: str
    title: str
    page_count: int
    checksum: str
    indexed_at: str


@dataclass
class Document:
    id: str
    metadata: DocumentMetadata
    elements: list


# ─── Extracción ───────────────────────────────────────────────────────────────

def extract(pdf_path: str) -> Document:
    doc = fitz.open(pdf_path)
    all_elements = []

    for page_num in range(len(doc)):
        page = doc[page_num]
        page_elements = extract_page(page, page_num + 1)
        all_elements.extend(page_elements)

    # Asignar SectionPath acumulado
    all_elements = attach_section_path(all_elements)
    # Descartar headers/footers repetidos
    all_elements = filter_page_headers(all_elements)

    checksum = file_checksum(pdf_path)

    return Document(
        id=str(uuid.uuid4())[:16],
        metadata=DocumentMetadata(
            source=pdf_path,
            doc_type="pdf",
            title=Path(pdf_path).stem,
            page_count=len(doc),
            checksum=checksum,
            indexed_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        ),
        elements=all_elements,
    )


def extract_page(page: fitz.Page, page_num: int) -> list:
    """
    Extrae elementos de una página con detección de tablas.
    
    La diferencia respecto a go-fitz: usamos find_tables() que analiza
    las coordenadas de los bloques para detectar filas y columnas,
    incluso en tablas sin bordes visibles.
    """
    elements = []

    # 1. Detectar tablas y sus bounding boxes
    tables = page.find_tables()
    table_bboxes = [t.bbox for t in tables]

    # 2. Extraer bloques de texto ignorando las áreas de tablas
    text_dict = page.get_text("dict")
    body_size = compute_body_size(text_dict)

    for block in text_dict.get("blocks", []):
        if block.get("type") != 0:  # 0 = text block
            continue

        block_bbox = block["bbox"]

        # Si este bloque está dentro de una tabla, lo saltamos
        # (la tabla se procesa por separado con find_tables)
        if is_inside_any_table(block_bbox, table_bboxes):
            continue

        text = extract_block_text(block)
        if not text.strip():
            continue

        font_size = get_dominant_font_size(block)
        is_bold = get_is_bold(block)

        elem_type, level = classify_block(text, font_size, body_size, is_bold)

        # Detectar list items por prefijo
        if is_list_item(text):
            elements.append(Element(
                type="list_item",
                text=strip_list_prefix(text),
                page=page_num,
            ))
            continue

        elements.append(Element(
            type=elem_type,
            level=level,
            text=text.strip(),
            page=page_num,
        ))

    # 3. Procesar tablas detectadas e insertarlas en la posición correcta
    for table in tables:
        cells = table.extract()  # list[list[str]]
        if not cells:
            continue

        # Filtrar filas vacías
        cells = [[cell or "" for cell in row] for row in cells if any(row)]
        if not cells:
            continue

        markdown_table = cells_to_markdown(cells)

        # Encontrar la página (top de la bbox)
        elements.append(Element(
            type="table",
            text=markdown_table,
            cells=cells,
            page=page_num,
        ))

    # Ordenar por posición vertical (top de cada bloque)
    elements.sort(key=lambda e: e.page)

    return elements


# ─── Clasificación de bloques (espejo del algoritmo Go) ──────────────────────

def compute_body_size(text_dict: dict) -> float:
    """
    Calcula la moda de font-sizes: el más frecuente es el cuerpo del texto.
    Mismo algoritmo que bodyFontSize() en Go.
    """
    sizes = []
    for block in text_dict.get("blocks", []):
        if block.get("type") != 0:
            continue
        for line in block.get("lines", []):
            for span in line.get("spans", []):
                size = round(span.get("size", 0) * 2) / 2  # bucket a 0.5pt
                if size > 0:
                    sizes.append(size)

    if not sizes:
        return 11.0

    try:
        return statistics.mode(sizes)
    except statistics.StatisticsError:
        return statistics.median(sizes)


def classify_block(text: str, font_size: float, body_size: float, is_bold: bool):
    """
    Clasifica un bloque como heading (con level) o párrafo.
    Misma lógica de ratios que classifyByRatio() en Go.
    """
    ratio = font_size / body_size if body_size > 0 else 1.0
    short = len(text) < 150
    ends_with_period = text.rstrip().endswith(".")

    if ratio >= 1.5:
        return "heading", 1
    if ratio >= 1.25:
        return "heading", 2
    if ratio >= 1.1 and is_bold and short:
        return "heading", 3
    if is_bold and short and not ends_with_period:
        return "heading", 3

    return "paragraph", 0


def get_dominant_font_size(block: dict) -> float:
    """Retorna el font-size del span con más texto en el bloque."""
    best_size = 11.0
    best_len = 0
    for line in block.get("lines", []):
        for span in line.get("spans", []):
            text_len = len(span.get("text", ""))
            if text_len > best_len:
                best_len = text_len
                best_size = span.get("size", 11.0)
    return best_size


def get_is_bold(block: dict) -> bool:
    """Detecta si el bloque dominante es bold por font flags o nombre de font."""
    for line in block.get("lines", []):
        for span in line.get("spans", []):
            flags = span.get("flags", 0)
            # Bit 4 de flags = bold en PyMuPDF
            if flags & 16:
                return True
            font_name = span.get("font", "").lower()
            if any(w in font_name for w in ("bold", "heavy", "black")):
                return True
    return False


def extract_block_text(block: dict) -> str:
    lines = []
    for line in block.get("lines", []):
        text = "".join(span.get("text", "") for span in line.get("spans", []))
        lines.append(text)
    return " ".join(lines)


# ─── List detection ───────────────────────────────────────────────────────────

import re

_BULLET_RE = re.compile(r"^[\s]*[•·◦▪▸▶\-\*]\s+")
_NUM_RE = re.compile(r"^[\s]*\d+[\.\)]\s+")
_ALPHA_RE = re.compile(r"^[\s]*[a-zA-Z][\.\)]\s+")


def is_list_item(text: str) -> bool:
    return bool(_BULLET_RE.match(text) or _NUM_RE.match(text) or _ALPHA_RE.match(text))


def strip_list_prefix(text: str) -> str:
    text = _BULLET_RE.sub("", text)
    text = _NUM_RE.sub("", text)
    text = _ALPHA_RE.sub("", text)
    return text.strip()


# ─── Table helpers ────────────────────────────────────────────────────────────

def cells_to_markdown(cells: list) -> str:
    """Convierte una tabla 2D a Markdown. Igual que rowsToMarkdown() en Go."""
    if not cells:
        return ""
    lines = []
    for i, row in enumerate(cells):
        lines.append("| " + " | ".join(str(c) for c in row) + " |")
        if i == 0:
            lines.append("|" + "|".join("---" for _ in row) + "|")
    return "\n".join(lines)


def is_inside_any_table(bbox, table_bboxes: list, margin: float = 2.0) -> bool:
    """Retorna True si el bbox está dentro de alguna tabla detectada."""
    x0, y0, x1, y1 = bbox
    for tx0, ty0, tx1, ty1 in table_bboxes:
        if (x0 >= tx0 - margin and y0 >= ty0 - margin and
                x1 <= tx1 + margin and y1 <= ty1 + margin):
            return True
    return False


# ─── SectionPath (espejo de attachSectionPath en Go) ─────────────────────────

def attach_section_path(elements: list) -> list:
    """
    Recorre los elementos en orden y asigna el SectionPath acumulado.
    Mismo algoritmo que attachSectionPath() en Go.
    """
    stack = [""] * 6  # stack[i] = título del heading de nivel i+1

    for el in elements:
        if el.type != "heading":
            level = next((i for i, s in enumerate(stack) if s), -1)
            if level >= 0:
                el.section_path = [s for s in stack[:level + 1] if s]
            continue

        lvl = max(1, min(6, el.level)) - 1
        stack[lvl] = el.text
        # Limpiar niveles más profundos
        for j in range(lvl + 1, 6):
            stack[j] = ""

        el.section_path = [s for s in stack[:lvl + 1] if s]

    return elements


def filter_page_headers(elements: list) -> list:
    """Descarta texto repetido en más de 3 páginas (headers/footers)."""
    from collections import Counter
    counts = Counter(el.text for el in elements)
    return [el for el in elements
            if not (counts[el.text] >= 3 and len(el.text) < 60)]


# ─── Utils ────────────────────────────────────────────────────────────────────

def file_checksum(path: str) -> str:
    sha = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            sha.update(chunk)
    return sha.hexdigest()


# ─── Serialización ────────────────────────────────────────────────────────────

def to_json(doc: Document) -> str:
    """
    Serializa el Document al JSON que Go puede leer directamente.
    Los campos vacíos se omiten (omitempty equivalente).
    """
    def clean(obj):
        if isinstance(obj, dict):
            return {k: clean(v) for k, v in obj.items()
                    if v not in (None, [], "", 0) or k in ("page", "level")}
        if isinstance(obj, list):
            return [clean(i) for i in obj]
        return obj

    raw = asdict(doc)
    return json.dumps(clean(raw), ensure_ascii=False, indent=2)


# ─── CLI ──────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Preprocesa un PDF a JSON estructurado para rag-go"
    )
    parser.add_argument("pdf", help="Path al archivo PDF")
    parser.add_argument(
        "--out", "-o",
        help="Path de salida del JSON (default: data/processed/<nombre>.json)"
    )
    args = parser.parse_args()

    if not os.path.exists(args.pdf):
        print(f"Error: no se encuentra el archivo '{args.pdf}'", file=sys.stderr)
        sys.exit(1)

    # Path de salida
    if args.out:
        out_path = args.out
    else:
        name = Path(args.pdf).stem
        out_path = f"data/processed/{name}.json"

    os.makedirs(os.path.dirname(out_path), exist_ok=True)

    print(f"Procesando {args.pdf}...")
    doc = extract(args.pdf)

    print(f"  {len(doc.elements)} elementos extraídos")
    tables = sum(1 for e in doc.elements if e.type == "table")
    headings = sum(1 for e in doc.elements if e.type == "heading")
    print(f"  {headings} headings, {tables} tablas")

    json_str = to_json(doc)
    with open(out_path, "w", encoding="utf-8") as f:
        f.write(json_str)

    print(f"  → {out_path}")
    print("Listo. Go leerá este archivo en la próxima indexación.")


if __name__ == "__main__":
    main()