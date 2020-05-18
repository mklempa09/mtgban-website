package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kodabb/go-mtgban/mtgban"
	"github.com/kodabb/go-mtgban/mtgdb"
	"github.com/kodabb/go-mtgban/mtgjson"
)

func Search(w http.ResponseWriter, r *http.Request) {
	sig := r.FormValue("Signature")
	exp := r.FormValue("Expires")

	signature := ""
	if sig != "" && exp != "" {
		signature = "?Signature=" + url.QueryEscape(sig) + "&Expires=" + url.QueryEscape(exp)
	}

	pageVars := PageVars{
		Title:      "BAN Search",
		Signature:  sig,
		Expires:    exp,
		LastUpdate: LastUpdate.Format(time.RFC3339),
	}
	pageVars.Nav = make([]NavElem, len(DefaultNav))
	copy(pageVars.Nav, DefaultNav)

	mainNavIndex := 0
	for i := range pageVars.Nav {
		pageVars.Nav[i].Link += signature
		if pageVars.Nav[i].Name == "Search" {
			mainNavIndex = i
		}
	}
	pageVars.Nav[mainNavIndex].Active = true
	pageVars.Nav[mainNavIndex].Class = "active"

	data := fmt.Sprintf("%s%s%s", r.Method, exp, r.URL.Host)
	valid := signHMACSHA1Base64([]byte(os.Getenv("BAN_SECRET")), []byte(data))
	expires, err := strconv.ParseInt(exp, 10, 64)
	if !DevMode && (err != nil || valid != sig || expires < time.Now().Unix()) {
		pageVars.Title = "Unauthorized"
		pageVars.ErrorMessage = "Please double check your invitation link"

		render(w, "search.html", pageVars)
		return
	}

	if !DatabaseLoaded {
		pageVars.Title = "Great things are coming"
		pageVars.ErrorMessage = "Website is starting, please try again in a few minutes"

		render(w, "search.html", pageVars)
		return
	}

	query := r.FormValue("q")

	if query != "" {
		pageVars.SearchQuery = query
		pageVars.FoundSellers = map[mtgdb.Card][]mtgban.CombineEntry{}
		pageVars.FoundVendors = map[mtgdb.Card][]mtgban.CombineEntry{}

		filterEdition := ""
		if strings.Contains(query, "s:") {
			fields := strings.Fields(query)
			for _, field := range fields {
				if strings.HasPrefix(field, "s:") {
					query = strings.TrimPrefix(query, field)
					query = strings.TrimSuffix(query, field)
					query = strings.TrimSpace(query)

					code := strings.TrimPrefix(field, "s:")
					filterEdition, _ = mtgdb.EditionCode2Name(code)
					break
				}
			}
		}
		for _, seller := range BanClient.Sellers() {
			inventory, err := seller.Inventory()
			if err != nil {
				log.Println(err)
				continue
			}
			for card, entries := range inventory {
				if mtgjson.NormPrefix(card.Name, query) {
					if filterEdition != "" && filterEdition != card.Edition {
						continue
					}
					for _, entry := range entries {
						if entry.Conditions != "NM" {
							continue
						}
						_, found := pageVars.FoundSellers[card]
						if !found {
							pageVars.FoundSellers[card] = []mtgban.CombineEntry{}
						}
						res := mtgban.CombineEntry{
							ScraperName: seller.Info().Name,
							Price:       entry.Price,
							Quantity:    entry.Quantity,
							URL:         entry.URL,
						}
						pageVars.FoundSellers[card] = append(pageVars.FoundSellers[card], res)
					}
				}
			}
		}

		for _, vendor := range BanClient.Vendors() {
			buylist, err := vendor.Buylist()
			if err != nil {
				log.Println(err)
				continue
			}
			for card, entry := range buylist {
				if filterEdition != "" && filterEdition != card.Edition {
					continue
				}
				if mtgjson.NormPrefix(card.Name, query) {
					_, found := pageVars.FoundVendors[card]
					if !found {
						pageVars.FoundVendors[card] = []mtgban.CombineEntry{}
					}
					res := mtgban.CombineEntry{
						ScraperName: vendor.Info().Name,
						Price:       entry.BuyPrice,
						Ratio:       entry.PriceRatio,
						Quantity:    entry.Quantity,
						URL:         entry.URL,
					}
					pageVars.FoundVendors[card] = append(pageVars.FoundVendors[card], res)
				}
			}
		}

		if len(pageVars.FoundSellers) == 0 && len(pageVars.FoundVendors) == 0 {
			pageVars.InfoMessage = "No results found"
		}
	}

	render(w, "search.html", pageVars)
	runtime.GC()
}