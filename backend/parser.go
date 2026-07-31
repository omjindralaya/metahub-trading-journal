package backend

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ParseMT5CSV membaca file CSV history MT5 dan mengubahnya menjadi slice of Trade.
// Asumsi format kolom standar CSV ekspor MT5 atau Custom CSV.
func ParseMT5CSV(filePath string) ([]Trade, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Terkadang delimiter CSV dari sistem Windows adalah semicolon (;) atau koma (,)
	reader.Comma = ',' 
	reader.FieldsPerRecord = -1 // Ignore row length validation for now
	
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var trades []Trade
	// MT5 Date format usually: "2023.10.19 14:00:00" atau "YYYY.MM.DD HH:MM:SS"
	layout := "2006.01.02 15:04:05"

	// Skip header jika baris pertama bukan angka/waktu
	startIndex := 0
	if len(records) > 0 && !strings.Contains(records[0][0], "20") {
		startIndex = 1 // Skip header row
	}

	for i := startIndex; i < len(records); i++ {
		row := records[i]
		if len(row) < 10 {
			continue // Skip baris tidak lengkap
		}

		// Asumsi pemetaan kolom (Bisa disesuaikan dengan format eksak CSV pengguna):
		// 0: Ticket, 1: OpenTime, 2: Type, 3: Volume, 4: Symbol, 5: OpenPrice, 6: S/L, 7: T/P, 
		// 8: CloseTime, 9: ClosePrice, 10: Commission, 11: Swap, 12: Profit
		// Jika format berbeda, logic mapping di bawah perlu diubah.
		
		ticket := row[0]
		openTime, _ := time.Parse(layout, row[1])
		
		// Parsing Type, Volume, Symbol
		tradeType := strings.ToLower(row[2]) // "buy" atau "sell"
		volume, _ := strconv.ParseFloat(row[3], 64)
		symbol := row[4]
		
		openPrice, _ := strconv.ParseFloat(row[5], 64)
		
		// Pastikan index mencukupi sebelum mem-parsing Close Time dkk (tergantung kolom CSV)
		// Kita gunakan fallback index jika format kolom standar
		closeTimeStr := ""
		if len(row) > 8 { closeTimeStr = row[8] }
		closeTime, _ := time.Parse(layout, closeTimeStr)
		
		closePrice, _ := strconv.ParseFloat(row[9], 64)
		
		commission, _ := strconv.ParseFloat(row[10], 64)
		swap, _ := strconv.ParseFloat(row[11], 64)
		profit, _ := strconv.ParseFloat(row[12], 64)

		// Filter hanya valid closed trades
		if symbol == "" || openTime.IsZero() || closeTime.IsZero() {
			continue
		}

		trade := Trade{
			Ticket:     ticket,
			OpenTime:   openTime,
			CloseTime:  closeTime,
			Symbol:     symbol,
			Type:       tradeType,
			Volume:     volume,
			OpenPrice:  openPrice,
			ClosePrice: closePrice,
			Commission: commission,
			Swap:       swap,
			Profit:     profit,
			NetProfit:  profit + commission + swap,
		}
		
		trades = append(trades, trade)
	}

	if len(trades) == 0 {
		return nil, fmt.Errorf("no valid trades found in file")
	}

	return trades, nil
}
