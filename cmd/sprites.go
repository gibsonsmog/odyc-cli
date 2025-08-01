package cmd

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var assetsPath string
var outputPath string
var force bool
var spriteSheet bool
var blockWidth int
var blockHeight int

func init() {
	spritesCmd.Flags().StringVarP(&assetsPath, "assets", "a", "", "path to assets directory")
	spritesCmd.Flags().StringVarP(&outputPath, "output", "o", "", "path to output file")
	spritesCmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite output file if it exists")
	spritesCmd.Flags().BoolVar(&spriteSheet, "sprite-sheet", false, "treat assets as a sprite sheet (multiple sprites in one image)")
	spritesCmd.Flags().IntVar(&blockWidth, "block-width", 0, "width of each sprite block (for sprite sheets)")
	spritesCmd.Flags().IntVar(&blockHeight, "block-height", 0, "height of each sprite block (for sprite sheets)")

	err := spritesCmd.MarkFlagRequired("assets")
	if err != nil {
		fmt.Println("Error marking flag required:", err)
	}

	err = spritesCmd.MarkFlagRequired("output")
	if err != nil {
		fmt.Println("Error marking flag required:", err)
	}

	rootCmd.AddCommand(spritesCmd)
}

var spritesCmd = &cobra.Command{
	Use:   "sprites [OPTIONS]",
	Short: "Generate code from sprites directory",
	Long:  `Output JavaScript file containing definitions for colors and sprites based on multiple images in assets directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		if _, err := os.Stat(filepath.Dir(outputPath)); err != nil {
			log.Error("Directory of output path does not exist")
			return
		}

		if _, err := os.Stat(outputPath); err == nil {
			if !force {
				log.Warn("Output file already exists. Please remove it first, or add --force flag to overwrite it")
				return
			}
		}

		if _, err := os.Stat(assetsPath); err != nil {
			log.Error("Assets directory does not exist")
			return
		}

		files, err := os.ReadDir(assetsPath)
		if err != nil {
			log.Error("Error reading assets directory: " + err.Error())
			return
		}

		pngs := make([]string, 0)

		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".png") {
				pngs = append(pngs, file.Name())
				continue
			}

			if file.IsDir() {
				log.Warn("Assets directory contains a directory: " + file.Name())
				continue
			}

			log.Info("Skipping non-PNG file: " + file.Name())
		}

		if len(pngs) == 0 {
			log.Error("No PNG files found in assets directory")
			return
		}

		type ColorMetadata struct {
			Color string
			Count int
			Files []string
			Index int
		}

		type SpriteMetadata struct {
			Rows [][]string
		}
		type TilesMetadata struct {
			Rows [][]string
		}

		sprites := map[string]SpriteMetadata{}
		tiles := map[string]TilesMetadata{}
		colors := map[string]ColorMetadata{}
		currentColorIndex := 0
		maxWidth := 0
		maxHeight := 0
		warnedAboutWidth := false
		warnedAboutHeight := false

		// Helper function to process a color and update the colors map
		processColor := func(hexCodeRGBA, png string) {
			if hexCodeRGBA != "#00000000" {
				if _, exists := colors[hexCodeRGBA]; !exists {
					colors[hexCodeRGBA] = ColorMetadata{
						Color: hexCodeRGBA,
						Count: 1,
						Files: []string{png},
						Index: currentColorIndex,
					}
					currentColorIndex++
				} else {
					color := colors[hexCodeRGBA]
					color.Count++
					filePresent := false
					for _, file := range color.Files {
						if file == png {
							filePresent = true
							break
						}
					}
					if !filePresent {
						color.Files = append(colors[hexCodeRGBA].Files, png)
					}
					colors[hexCodeRGBA] = color
				}
			}
		}

		// Helper function to get color index character for a pixel
		getColorIndex := func(hexCodeRGBA string) string {
			colorIndexOfPixel := "."
			if hexCodeRGBA != "#00000000" {
				if colors[hexCodeRGBA].Index < 10 {
					colorIndexOfPixel = strconv.Itoa(colors[hexCodeRGBA].Index)
				} else {
					newIndex := colors[hexCodeRGBA].Index - 10
					charsMap := strings.Split("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", "")
					if newIndex-1 > len(charsMap) {
						log.Error("Too many colors. You can only use up to 62 colors")
					}
					colorIndexOfPixel = charsMap[newIndex]
				}
			}
			return colorIndexOfPixel
		}

		// Helper function to process a single pixel
		processPixel := func(img image.Image, x, y int, png string) string {
			c := img.At(x, y)
			r, g, b, a := c.RGBA()
			r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8)
			hexCodeRGBA := fmt.Sprintf("#%02x%02x%02x%02x", r8, g8, b8, a8)
			processColor(hexCodeRGBA, png)
			return getColorIndex(hexCodeRGBA)
		}

		for _, png := range pngs {
			spriteName := strings.TrimSuffix(png, ".png")
			file, err := os.Open(filepath.Join(assetsPath, png))
			if err != nil {
				log.Error("Error opening file " + png + ": " + err.Error())
				continue
			}
			defer func() {
				if err := file.Close(); err != nil && err == nil {
					panic(err)
				}
			}()

			img, _, err := image.Decode(file)
			if err != nil {
				log.Error("Error decoding image: " + err.Error())
				return
			}

			bounds := img.Bounds()

			// If blockWidth and blockHeight are set, treat as sprite sheet
			if spriteSheet && blockWidth > 0 && blockHeight > 0 {
				blocksX := bounds.Dx() / blockWidth
				blocksY := bounds.Dy() / blockHeight
				if bounds.Dx()%blockWidth != 0 || bounds.Dy()%blockHeight != 0 {
					log.Warn("Image " + png + " dimensions are not a multiple of block size; some pixels may be ignored.")
				}
				for by := 0; by < blocksY; by++ {
					for bx := 0; bx < blocksX; bx++ {
						blockSpriteName := spriteName + "_" + strconv.Itoa(by) + "_" + strconv.Itoa(bx)
						if _, exists := tiles[blockSpriteName]; !exists {
							tiles[blockSpriteName] = TilesMetadata{
								Rows: make([][]string, blockHeight),
							}
						}
						for y := 0; y < blockHeight; y++ {
							spriteRow := make([]string, blockWidth)
							for x := 0; x < blockWidth; x++ {
								ix := bx*blockWidth + x
								iy := by*blockHeight + y
								if ix >= bounds.Dx() || iy >= bounds.Dy() {
									spriteRow[x] = "."
									continue
								}
								spriteRow[x] = processPixel(img, ix, iy, png)
							}
							tiles[blockSpriteName].Rows[y] = spriteRow
						}
					}
				}
				if blockWidth > maxWidth {
					if maxWidth != 0 && !warnedAboutWidth {
						log.Warn("Block width is larger than previous max width")
						warnedAboutWidth = true
					}
					maxWidth = blockWidth
				}
				if blockHeight > maxHeight {
					if maxHeight != 0 && !warnedAboutHeight {
						log.Warn("Block height is larger than previous max height")
						warnedAboutHeight = true
					}
					maxHeight = blockHeight
				}
				continue // skip normal sprite logic
			}

			if bounds.Dx() > maxWidth {
				if maxWidth != 0 && !warnedAboutWidth {
					log.Warn("Images have different widths, which usually indicate a sprite sheet problem")
					warnedAboutWidth = true
				}
				maxWidth = bounds.Dx()
			}

			if bounds.Dy() > maxHeight {
				if maxHeight != 0 && !warnedAboutHeight {
					log.Warn("Images have different heights, which usually indicate a sprite sheet problem")
					warnedAboutHeight = true
				}

				maxHeight = bounds.Dy()
			}

			if _, exists := sprites[spriteName]; !exists {
				sprites[spriteName] = SpriteMetadata{
					Rows: make([][]string, bounds.Max.Y),
				}
			}

			rowI := 0
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				spriteRow := make([]string, bounds.Max.X)

				columnI := 0
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					spriteRow[columnI] = processPixel(img, x, y, png)
					columnI++
				}

				sprites[spriteName].Rows[rowI] = spriteRow
				rowI++
			}
		}

		if len(colors) == 0 {
			log.Error("No colors found in PNG images")
			return
		}

		if len(sprites) == 0 && !spriteSheet {
			log.Error("No sprites made from PNG images")
			return
		}
		if len(tiles) == 0 && spriteSheet {
			log.Error("No tiles made from PNG images")
			return
		}

		for _, color := range colors {
			word := "file"
			if color.Count > 1 {
				word = "files"
			}
			log.Debug(color.Color + " found " + strconv.Itoa(color.Count) + " times in " + strconv.Itoa(len(color.Files)) + " " + word)
		}

		log.Info(strconv.Itoa(len(colors)) + " colors found across all sprites")
		if !spriteSheet {
			log.Info(strconv.Itoa(len(sprites)) + " sprites found across all PNG files")
		} else {
			log.Info(strconv.Itoa(len(tiles)) + " tiles found across all PNG files")
		}

		codeColors := make([]string, len(colors))
		{
			for hex, color := range colors {
				codeColors[color.Index] = hex
			}
		}

		codeSprites := make([]string, len(sprites))
		{
			i := 0
			for spriteName, sprite := range sprites {
				rows := sprite.Rows

				codeRows := make([]string, len(rows))
				for rowI, row := range rows {
					codeRows[rowI] = `			` + strings.Join(row, "")
				}

				codeSprites[i] = `		"` + spriteName + `": ` + "`" + `
` + strings.Join(codeRows, "\n") + `
		` + "`,"
				i++
			}
		}
		// This is set up this way to hopefully match the style of the codeSprites declaration above and the sprites object set up below. Probably warrants a refactor
		var codeTiles []string
		{
			// group tiles by name
			// this feels somewhat clunky
			tileGroups := make(map[string]map[string]map[string]string)
			for spriteName, sprite := range tiles {
				tileVals := strings.Split(spriteName, "_")
				tileName := tileVals[0]
				tileRow := tileVals[1]
				tileColumn := tileVals[2]

				if tileGroups[tileName] == nil {
					tileGroups[tileName] = make(map[string]map[string]string)
				}
				if tileGroups[tileName][tileRow] == nil {
					tileGroups[tileName][tileRow] = make(map[string]string)
				}

				codeRows := make([]string, len(sprite.Rows))
				for rowI, row := range sprite.Rows {
					codeRows[rowI] = `            ` + strings.Join(row, "")
				}

				tileGroups[tileName][tileRow][tileColumn] = "`\n" + strings.Join(codeRows, "\n") + "\n            `"
			}

			// output formatting
			// this doesnt feel clunky, it is clunky
			codeTiles = make([]string, 0, len(tileGroups))
			for tileName, rows := range tileGroups {
				rowStrings := make([]string, 0, len(rows))
				for row, columns := range rows {
					columnStrings := make([]string, 0, len(columns))
					for col, content := range columns {
						columnStrings = append(columnStrings, fmt.Sprintf(`"%s": %s`, col, content))
					}
					rowStrings = append(rowStrings, fmt.Sprintf(`        "%s": {%s}`,
						row,
						strings.Join(columnStrings, ","),
					))
				}
				codeTiles = append(codeTiles, fmt.Sprintf(`    "%s": {
		%s
			},`, tileName, strings.Join(rowStrings, ",\n")))
			}
		}

		// TODO: This is ugly, use some templating engine. Your future self will thank you a lot.
		code := `var gameConfig = {
	cellWidth: ` + strconv.Itoa(maxWidth) + `,
	cellHeight: ` + strconv.Itoa(maxHeight) + `,
	colors: [
		"` + strings.Join(codeColors, `",
		"`) + `",
	],
	tiles: {
	` + strings.Join(codeTiles, "\n") + `
	},
	sprites: {
` + strings.Join(codeSprites, "\n") + `
	}
};`

		// wrtie code to outputPAth
		if outputPath != "" {
			err := os.WriteFile(outputPath, []byte(code), 0644)
			if err != nil {
				log.Errorf("Failed to write code to output file: %v", err)
			}
		}
		if spriteSheet {
			log.Info("Tileset configuration generated successfully")
		} else {
			log.Logf(2, "Sprites configuration generated successfully")
		}
	},
}
