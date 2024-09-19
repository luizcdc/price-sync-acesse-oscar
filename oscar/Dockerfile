FROM golang:alpine
ENV PYTHONUNBUFFERED=1
LABEL authors="Luiz Cavalcanti"

WORKDIR /app/sync-acesse-oscar/oscar

COPY go.mod go.sum ./
COPY .env ./

RUN go mod download

#RUN apk update && apk add --no-cache git

COPY . .

EXPOSE 8000

CMD ["go", "run", "."]
