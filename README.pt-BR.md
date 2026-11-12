# EngTree Viewer

[English](README.md) · **Português (Brasil)**

Um visualizador leve e rápido de BOMs de engenharia. Abra um produto, veja a lista de materiais em árvore, clique em qualquer peça e o desenho, o DXF ou o arquivo 3D dela abre ao lado.

Feito para o chão de fábrica: um único `.exe` portátil, sem instalação, sem banco de dados, sem conta. Ele lê as planilhas e pastas que você já tem (locais ou na rede) e nunca altera nada. Roda bem em computadores antigos, onde visualizadores CAD mais pesados sofrem.

## Recursos

- **Árvore da BOM** pelos números de item no padrão SolidWorks (1, 1.1, 1.1.1), com expandir/recolher, filtro e navegação pelo teclado
- **Arquivos ligados a cada peça**: cada linha mostra quais tipos de arquivo existem para ela (PDF, DXF, IGES…)
- **Visualizadores embutidos**: PDF, DXF (2D) e IGES (3D sombreado) com zoom, arrasto, rotação e **medição**; imagens e arquivos de texto/CNC também. O resto abre no programa padrão
- **Revisões**: quando vários arquivos batem com a peça (`ABC-100.pdf`, `ABC-100_rev2.pdf`), escolha em uma lista; nome exato primeiro, depois o mais novo
- **Aba Peças e Conjuntos**: todas as peças de todas as BOMs em uma lista pesquisável, com onde cada uma é usada, filtros por colunas da planilha e por tipo de arquivo; abrir um conjunto mostra a subárvore dele
- **Lista de arquivos para programas de máquina**: marque um tipo de arquivo como *Lista* (ex.: CNC com `.mpr, .nc, .tap`…) para ver todos os arquivos da peça com data e tamanho e baixar um a um ou em ZIP
- **Anexos** (📎): fotos, vídeos e documentos guardados em uma pasta ou arquivo com o nome do código
- **Pacote ZIP**: todos os PDFs (ou DXFs…) de um produto em um clique
- **Suas planilhas, suas colunas**: `.xlsx`, `.csv` (vírgula ou ponto e vírgula) ou `.txt` separado por tabulação. Só quatro colunas precisam ser encontradas (item, código, descrição, quantidade); as demais aparecem como estão
- **Planilha de detalhes opcional**, ligada pelo código da peça (acabamento, cor, fornecedor…)
- **Tipos de arquivo configuráveis**: nome, extensões, uma ou mais pastas e nome exato ou por prefixo
- **Interface em português e inglês**: segue o idioma do sistema, ou escolha nas Configurações
- **Funciona offline**: se a rede cair, uma barra vermelha avisa e a lista de produtos continua disponível

## Requisitos

- Windows 10 ou 11
- Microsoft Edge ou Google Chrome
- Para compilar: [Go](https://go.dev/dl/) 1.21+

## Como começar

1. Baixe o `EngTreeViewer.exe` em [Releases](https://github.com/ALEXKAKIUCHI/engtree-viewer/releases) ou compile com `build.bat`.
2. Abra o programa. Na primeira vez ele pede as pastas.
3. Em **Configurações**, informe a **pasta das BOMs** (cada planilha é um produto), o **prefixo** dos arquivos de BOM, se houver (ex.: `BOM - `), e as pastas de cada **tipo de arquivo**.

### Nomes dos arquivos

| O quê | Exemplo | Observação |
|---|---|---|
| BOM | `BOM - ABC-100 - Mesa lateral.xlsx` | código e descrição vêm do nome, separados por ` - ` |
| Detalhes (opcional) | `PECAS - ABC-100.xlsx` | mesma pasta da BOM, ligada pelo código |
| Arquivo da peça, exato | `ABC-110.dxf` | nome = código |
| Arquivo da peça, prefixo | `ABC-110 - Perna.pdf`, `ABC-110_rev2.pdf` | `ABC-1100.pdf` **não** casa com `ABC-110` |

No modo prefixo, um arquivo não aparece como revisão de uma peça quando o nome dele é o código de outra peça do mesmo produto: se a BOM tem `ABC-110` e `ABC-110-01`, o arquivo `ABC-110-01.pdf` pertence só à `ABC-110-01`.

### Configurações e modo portátil

As configurações ficam em `%APPDATA%\EngTreeViewer\engtree.json`. Se existir um `engtree.json` **ao lado do .exe**, ele é usado no lugar, o que facilita copiar a mesma configuração para vários computadores.

## Privacidade

O EngTree Viewer só **lê** as pastas configuradas e nunca grava nelas; roda um servidor local em `127.0.0.1`, que só entrega arquivos dessas pastas e recusa pedidos vindos de outros sites; **não envia nada pela internet e não coleta dados**. "Reportar um problema" apenas abre a página do projeto no GitHub.

## Limitações

- O Excel antigo `.xls` binário não é lido: salve como `.xlsx` ou `.csv`. (Exportações em texto que só têm a extensão `.xls` são lidas.)
- Faces curvas do IGES aparecem facetadas; é um visualizador rápido, não um CAD.
- A hierarquia vem do número do item (1.2.1). BOMs com coluna de "nível" ainda não são suportadas.

## Licença

[MIT](LICENSE) © Alexsander Kakiuchi
