FROM golang:1.20-alpine

ENV BOT_TOKEN = ""
ENV NUTRITIONIX_API_KEY = ""
ENV NUTRITIONIX_APP_ID = ""
ENV OPENAI_TOKEN =""

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download
COPY *.go ./
RUN go build -o /sugarsnapbot

EXPOSE 8080

CMD [ "/sugarsnapbot" ]